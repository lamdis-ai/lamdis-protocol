// Calls the live sandbox through the tools exactly as generateText would.
// No account, no key, no model key, no money.
//
//   npm run smoke          (from integrations/vercel-ai)
import assert from "node:assert/strict";
import { lamdisCheckFeasible, lamdisJobStatus, lamdisRunJob, lamdisTools } from "@lamdis/ai-sdk-tools";

const AT = { predicate: "the sign is up at the front", lat: 42.3314, lon: -83.0458 };
const call = { toolCallId: "smoke", messages: [] };
const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

async function main() {
  assert.deepEqual(Object.keys(lamdisTools), ["lamdis_check_feasible", "lamdis_run_job", "lamdis_job_status"]);
  console.log("ok  tools are AI SDK tools");

  const qs = (await lamdisCheckFeasible.execute!({ ...AT, kind: "observe", sandbox: true }, call)) as any;
  assert.equal(qs.sandbox, true); assert.equal(qs.feasible, true); assert.equal(qs.reachable, "simulated");
  console.log("ok  check_feasible sandbox");

  const ql = (await lamdisCheckFeasible.execute!({ ...AT, kind: "observe", sandbox: false }, call)) as any;
  assert.ok(!("sandbox" in ql) && typeof ql.feasible === "boolean", JSON.stringify(ql));
  assert.ok(["none", "a few", "several", "plenty"].includes(ql.reachable));
  console.log("ok  check_feasible live is honest");

  const posted = (await lamdisRunJob.execute!({ ...AT, fee_minor: 800, kind: "observe", radius_m: 150, sandbox: true }, call)) as any;
  assert.equal(posted.sandbox, true); assert.equal(posted.escrowed, 0);
  assert.ok(posted.job && String(posted.token).startsWith("lbt_"), JSON.stringify(posted));
  let status: any;
  for (let i = 0; i < 20; i++) {
    status = await lamdisJobStatus.execute!({ job: posted.job, token: posted.token }, call);
    if ((status.submissions ?? 0) >= 1) break;
    await sleep(1500);
  }
  assert.equal(status.sandbox, true); assert.equal(status.taken, 1);
  assert.equal(status.results[0].verified, true); assert.match(status.note, /SANDBOX/);
  console.log("ok  run_job sandbox walks the loop");

  const bad = (await lamdisJobStatus.execute!({ job: "observe-0", token: "lbt_nope" }, call)) as any;
  assert.ok([401, 404].includes(bad.http_status) && bad.error, JSON.stringify(bad));
  console.log("ok  bad token is an error not a throw");
  console.log("all passed");
}
main().catch((e) => { console.error(e); process.exit(1); });
