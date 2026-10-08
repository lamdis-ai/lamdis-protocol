"""Drop mode-only file sections (old mode/new mode with no content change)
from a predictions file, writing <name>.clean.jsonl."""
import json, re, sys

def clean(patch):
    parts = re.split(r'(?=^diff --git )', patch, flags=re.M)
    keep = [p for p in parts if p.strip() and not re.fullmatch(r'diff --git [^\n]*\nold mode \d+\nnew mode \d+\n?', p)]
    return "".join(keep)

src = sys.argv[1]
out = src.replace(".jsonl", ".clean.jsonl")
with open(out, "w") as f:
    for l in open(src):
        x = json.loads(l)
        x["model_patch"] = clean(x["model_patch"])
        f.write(json.dumps(x) + "\n")
print(out)
