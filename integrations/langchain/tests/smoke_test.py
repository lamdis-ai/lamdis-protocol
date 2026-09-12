"""Calls the live sandbox. No account, no key, no money.

    python tests/smoke_test.py
"""

import time

from lamdis_langchain import lamdis_check_feasible, lamdis_job_status, lamdis_run_job, lamdis_tools

AT = {"predicate": "the sign is up at the front", "lat": 42.3314, "lon": -83.0458}


def test_tools_are_langchain_tools():
    names = [t.name for t in lamdis_tools]
    assert names == ["lamdis_check_feasible", "lamdis_run_job", "lamdis_job_status"]
    assert "sandbox" in lamdis_run_job.args and lamdis_run_job.args["sandbox"]["default"] is True


def test_check_feasible_sandbox():
    q = lamdis_check_feasible.invoke({**AT, "sandbox": True})
    assert q["sandbox"] is True and q["feasible"] is True and q["reachable"] == "simulated", q


def test_check_feasible_live_is_honest():
    q = lamdis_check_feasible.invoke(AT)
    assert "sandbox" not in q and isinstance(q["feasible"], bool), q
    assert q["reachable"] in ("none", "a few", "several", "plenty"), q
    if not q["feasible"]:
        assert q["why"] and q["advice"], q


def test_run_job_sandbox_walks_the_loop():
    posted = lamdis_run_job.invoke({**AT, "fee_minor": 800})
    assert posted.get("sandbox") is True and posted.get("escrowed") == 0, posted
    assert posted["job"] and posted["token"].startswith("lbt_"), posted
    status = None
    for _ in range(20):
        status = lamdis_job_status.invoke({"job": posted["job"], "token": posted["token"]})
        if status.get("submissions", 0) >= 1:
            break
        time.sleep(1.5)
    assert status["sandbox"] is True and status["taken"] == 1, status
    assert status["results"][0]["verified"] is True, status
    assert "SANDBOX" in status["note"]


def test_bad_token_is_an_error_not_an_exception():
    out = lamdis_job_status.invoke({"job": "observe-0", "token": "lbt_nope"})
    assert out["http_status"] in (401, 404) and "error" in out, out


if __name__ == "__main__":
    for name, fn in list(globals().items()):
        if name.startswith("test_"):
            fn()
            print("ok ", name)
    print("all passed")
