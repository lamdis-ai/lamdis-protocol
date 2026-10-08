"""Run the lamdis agent on SWE-bench Verified instances and collect patches.

    .venv/bin/python run_gen.py [-n 50] [-j 4] [--model z-ai/glm-5.3-flash] [--cap 5]

Each instance gets a fresh checkout at its base commit, the agent gets the
issue text, and whatever it changed becomes the prediction. Spend is read
from the OpenRouter key's own usage counter, and every running agent is
killed once the run has spent more than --cap dollars.
"""
import argparse, json, os, shutil, subprocess, threading, time, urllib.request
from concurrent.futures import ThreadPoolExecutor, as_completed

HERE = os.path.dirname(os.path.abspath(__file__))
BIN = os.path.join(HERE, ".bin", "lamdis")
DATA = os.path.join(HERE, ".lamdis-data")
REPOS = os.path.join(HERE, ".repos")
WORK = os.path.join(HERE, ".work")

PROMPT = """Here is an issue reported against this repository:

<issue>
{issue}
</issue>

Resolve the issue by editing the non-test source files in this repository so the described problem is fixed. Do not modify or add tests. The project's dependencies may not be installed, so reason from the code if you cannot run it."""


def env_key():
    for line in open(os.path.expanduser("~/.lamdis/.env")):
        if line.startswith("LAMDIS_OPENROUTER_KEY="):
            return line.split("=", 1)[1].strip().strip('"')
    raise SystemExit("no LAMDIS_OPENROUTER_KEY in ~/.lamdis/.env")


def usage(key):
    req = urllib.request.Request("https://openrouter.ai/api/v1/key", headers={"Authorization": "Bearer " + key})
    return json.load(urllib.request.urlopen(req, timeout=30))["data"]["usage"]


def setup_data(model, data=DATA):
    """A data dir of its own, so the bench never lands in the person's record.
    Each instance gets a fresh one: lamdis files every task in a thread named
    after the repository, so a shared one lets one task read another's."""
    os.makedirs(data, exist_ok=True)
    src = os.path.expanduser("~/.lamdis")
    for f in ("person.key", "agent.key", "name", ".env"):
        if os.path.exists(os.path.join(src, f)) and not os.path.exists(os.path.join(data, f)):
            shutil.copy(os.path.join(src, f), os.path.join(data, f))
    cfg = {"model": model, "trust": "project", "unguarded": True, "max_tool_calls": 200,
           "max_runs_per_day": 100000, "max_fetches_per_day": 100000, "max_tokens_per_day": 1000000000}
    json.dump(cfg, open(os.path.join(data, "agent.json"), "w"), indent=2)


_clone_locks = {}
_lock = threading.Lock()


def checkout(inst):
    repo = inst["repo"]
    with _lock:
        lk = _clone_locks.setdefault(repo, threading.Lock())
    mirror = os.path.join(REPOS, repo.replace("/", "__"))
    with lk:
        if not os.path.exists(mirror):
            subprocess.run(["git", "clone", "-q", "--bare", f"https://github.com/{repo}.git", mirror], check=True)
    d = os.path.join(WORK, inst["instance_id"])
    shutil.rmtree(d, ignore_errors=True)
    subprocess.run(["git", "clone", "-q", "--shared", "--no-checkout", mirror, d], check=True)
    subprocess.run(["git", "-c", "advice.detachedHead=false", "checkout", "-q", inst["base_commit"]], cwd=d, check=True)
    return d


def diff(d, base):
    subprocess.run(["git", "add", "-A"], cwd=d, check=True)
    return subprocess.run(["git", "diff", "--cached", base], cwd=d, capture_output=True, text=True).stdout


procs = set()
stop = threading.Event()


def solve(inst, model, timeout, logdir):
    if stop.is_set():
        return None
    t0 = time.time()
    d = checkout(inst)
    data = os.path.join(DATA, "runs", inst["instance_id"])
    shutil.rmtree(data, ignore_errors=True)
    setup_data(model, data)
    log = open(os.path.join(logdir, inst["instance_id"] + ".log"), "w")
    # Offline: the score has to come from the repository, not from finding
    # the upstream fix on the web.
    p = subprocess.Popen([BIN, "-data", data, "-q", "-offline", "-dir", d, PROMPT.format(issue=inst["problem_statement"])],
                         cwd=d, stdout=log, stderr=subprocess.STDOUT, env={**os.environ, "LAMDIS_MODEL": model})
    procs.add(p)
    try:
        p.wait(timeout=timeout)
        status = "done" if p.returncode == 0 else f"exit {p.returncode}"
    except subprocess.TimeoutExpired:
        p.kill()
        status = "timeout"
    procs.discard(p)
    if stop.is_set() and status != "done":
        status = "killed (cap)"
    patch = diff(d, inst["base_commit"])
    return {"instance_id": inst["instance_id"], "model_name_or_path": "lamdis+" + model,
            "model_patch": patch, "status": status, "seconds": round(time.time() - t0)}


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("-n", type=int, default=50)
    ap.add_argument("-j", type=int, default=4)
    ap.add_argument("--model", default="z-ai/glm-5.3-flash")
    ap.add_argument("--cap", type=float, default=5.0, help="dollars; abort past this")
    ap.add_argument("--timeout", type=int, default=1500, help="seconds per instance")
    ap.add_argument("--sample", default=os.path.join(HERE, "sample50.json"))
    ap.add_argument("--ids", default="", help="comma-separated instance ids to run (default: the first -n)")
    ap.add_argument("--tag", default="", help="suffix for the results directory, to keep runs apart")
    a = ap.parse_args()

    insts = json.load(open(a.sample))
    if a.ids:
        want = set(a.ids.split(","))
        insts = [i for i in insts if i["instance_id"] in want]
    else:
        insts = insts[: a.n]
    tag = a.model.replace("/", "_") + (("-" + a.tag) if a.tag else "")
    out = os.path.join(HERE, "results", tag)
    os.makedirs(os.path.join(out, "logs"), exist_ok=True)
    preds_path = os.path.join(out, "predictions.jsonl")
    done = set()
    if os.path.exists(preds_path):
        done = {json.loads(l)["instance_id"] for l in open(preds_path)}
    todo = [i for i in insts if i["instance_id"] not in done]
    key = env_key()
    start = usage(key)
    print(f"{len(todo)} to run ({len(done)} already done), cap ${a.cap:.2f}", flush=True)

    with ThreadPoolExecutor(a.j) as ex, open(preds_path, "a") as preds:
        futs = [ex.submit(solve, i, a.model, a.timeout, os.path.join(out, "logs")) for i in todo]
        for n, f in enumerate(as_completed(futs), 1):
            r = f.result()
            spent = usage(key) - start
            if r:
                preds.write(json.dumps(r) + "\n")
                preds.flush()
                print(f"[{n}/{len(todo)}] {r['instance_id']}: {r['status']}, {r['seconds']}s, "
                      f"{len(r['model_patch'].splitlines())} diff lines · spent ${spent:.3f}", flush=True)
            if spent > a.cap and not stop.is_set():
                print(f"CAP HIT: ${spent:.2f} > ${a.cap:.2f}, stopping", flush=True)
                stop.set()
                for p in list(procs):
                    p.kill()
    print(f"total spent ${usage(key) - start:.3f}", flush=True)


if __name__ == "__main__":
    main()
