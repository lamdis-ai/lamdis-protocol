# SWE-bench Verified for the lamdis CLI

Runs `lamdis -offline` on a seeded 50-task sample of SWE-bench Verified and
grades the patches with the official harness.

    uv venv -p 3.12 .venv && uv pip install -p .venv swebench datasets
    (cd ../node && go build -o ../bench/.bin/lamdis ./cmd/lamdis)
    .venv/bin/python run_gen.py -n 50 -j 4 --cap 5            # resumable
    .venv/bin/python -m swebench.harness.run_evaluation -d SWE-bench/SWE-bench_Verified \
        -p results/<model>/predictions.jsonl --max_workers 3 -id <run> --report_dir results

On Apple Silicon the evaluation images are x86 only: pull each with
`docker pull --platform linux/amd64 <image>` first. Spend is read from the
OpenRouter key and the run stops past `--cap` dollars.
