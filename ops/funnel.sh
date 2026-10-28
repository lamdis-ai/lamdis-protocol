#!/bin/bash
# Who showed up, who typed, who came back: one line per day.
#   opened  = new guest starts (ios / web), from .starts.log
#   typed   = accounts whose first message was that day
#   back    = of those, how many wrote again on a later day
# Usage: ops/funnel.sh [days]   (default 14)
days=${1:-14}
ssh -o ConnectTimeout=10 -i ~/.ssh/shared-box.pem ec2-user@54.85.29.91 "sudo python3 - $days" <<'EOF'
import sqlite3, glob, json, sys, collections, datetime, os
root = "/mnt/lamdis/app-accounts/accounts"
days = int(sys.argv[1])
since = (datetime.date.today() - datetime.timedelta(days=days - 1)).isoformat()
human = ("core.message", "chat.question", "agent.decision_reply")
opened = collections.defaultdict(collections.Counter)
try:
    for line in open(os.path.join(root, ".starts.log")):
        ts, client = line.split()
        opened[ts[:10]][client] += 1
except FileNotFoundError:
    pass
typed, back = collections.Counter(), collections.Counter()
for db in glob.glob(root + "/*/lamdis.db"):
    con = sqlite3.connect("file:%s?mode=ro" % db, uri=True)
    seen = sorted({str(json.loads(r)["ts"])[:10] for (r,) in con.execute("select raw from entries")
                   if json.loads(r)["kind"] in human})
    if seen:
        typed[seen[0]] += 1
        if len(seen) > 1:
            back[seen[0]] += 1
print("%-10s %6s %6s %6s %6s" % ("day", "ios", "web", "typed", "back"))
d = datetime.date.fromisoformat(since)
while d <= datetime.date.today():
    k = d.isoformat()
    print("%-10s %6d %6d %6d %6d" % (k, opened[k]["ios"], opened[k]["web"], typed[k], back[k]))
    d += datetime.timedelta(days=1)
EOF
