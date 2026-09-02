import { describe, expect, it } from "vitest";
import { Lamdis, LamdisError } from "../src/index";

interface Call {
  method: string;
  url: string;
  headers: Record<string, string>;
  body: unknown;
}

/** A fetch that records what it was sent and answers from a table. */
function fakeFetch(routes: Record<string, { status?: number; body: unknown }>) {
  const calls: Call[] = [];
  const f: typeof fetch = async (input, init) => {
    const url = typeof input === "string" ? input : (input as URL).toString();
    const method = init?.method ?? "GET";
    const headers = Object.fromEntries(
      Object.entries((init?.headers ?? {}) as Record<string, string>),
    );
    const body = init?.body ? JSON.parse(init.body as string) : undefined;
    calls.push({ method, url, headers, body });
    const key = `${method} ${new URL(url).pathname}`;
    const route = routes[key];
    if (!route) {
      return new Response(JSON.stringify({ error: `no route ${key}` }), { status: 404 });
    }
    return new Response(JSON.stringify(route.body), {
      status: route.status ?? 200,
      headers: { "Content-Type": "application/json" },
    });
  };
  return { fetch: f, calls };
}

const awaiting = {
  job: "observe-1",
  kind: "observe",
  status: "awaiting_payment",
  pay_at: "https://checkout.example/cs_123",
  token: "lbt_abc",
  amount_minor: 800,
  currency: "USD",
  expires_at: "2026-09-03T00:00:00Z",
  status_url: "https://exchange.lamdis.ai/v1/jobs/observe-1",
  watch: "https://exchange.lamdis.ai/my/observe-1?t=lbt_abc",
  note: "Send the person the pay link.",
};

describe("anonymous first job", () => {
  it("observe() posts kind=observe with no credential and returns the pay link and token", async () => {
    const { fetch, calls } = fakeFetch({ "POST /v1/tasks": { body: awaiting } });
    const x = new Lamdis({ fetch });
    const posted = await x.observe({
      predicate: "The 'For Lease' sign is still up",
      where: "1200 Valencia St",
      lat: 37.75,
      lon: -122.42,
      radius_m: 150,
      fee_minor: 800,
    });
    expect(calls).toHaveLength(1);
    expect(calls[0].url).toBe("https://exchange.lamdis.ai/v1/tasks");
    expect(calls[0].headers["X-Lamdis-Key"]).toBeUndefined();
    expect(calls[0].headers["Authorization"]).toBeUndefined();
    expect(calls[0].headers["X-Lamdis-Posted-By"]).toBe("agent");
    expect(calls[0].body).toMatchObject({ kind: "observe", fee_minor: 800, predicate: "The 'For Lease' sign is still up" });
    expect(posted.job).toBe("observe-1");
    expect(posted.status).toBe("awaiting_payment");
    expect(posted.payAt).toBe(awaiting.pay_at);
    expect(posted.token).toBe("lbt_abc");
    expect(posted.watch).toBe(awaiting.watch);
    expect(posted.amountMinor).toBe(800);
    expect(posted.raw).toEqual(awaiting);
  });

  it("do() posts kind=do and carries instructions", async () => {
    const { fetch, calls } = fakeFetch({ "POST /v1/tasks": { body: { ...awaiting, job: "do-1", kind: "do" } } });
    const posted = await new Lamdis({ fetch }).do({
      predicate: "The bins are behind the gate",
      instructions: "Wheel both bins through the side gate.",
      fee_minor: 1200,
      attempt_minor: 300,
    });
    expect(calls[0].body).toMatchObject({ kind: "do", instructions: "Wheel both bins through the side gate.", attempt_minor: 300 });
    expect(posted.kind).toBe("do");
  });

  it("job(id, token) sends the token as a bearer on status, receipt, evidence, cancel, release, hold", async () => {
    const { fetch, calls } = fakeFetch({
      "GET /v1/jobs/observe-1": { body: { job: "observe-1", status: "awaiting_payment" } },
      "GET /v1/jobs/observe-1/receipt": { body: { job: "observe-1", accepted: true, signature: "aa" } },
      "GET /v1/jobs/observe-1/evidence": { body: { job: "observe-1", files: [] } },
      "POST /v1/jobs/observe-1/cancel": { body: { job: "observe-1", cancelled: true, released_minor: 800 } },
      "POST /v1/jobs/observe-1/release": { body: { job: "observe-1", released: 1, status: "ok" } },
      "POST /v1/jobs/observe-1/hold": { body: { job: "observe-1", held: 1 } },
    });
    const job = new Lamdis({ fetch }).job("observe-1", "lbt_abc");
    expect((await job.status()).status).toBe("awaiting_payment");
    expect((await job.receipt()).accepted).toBe(true);
    expect((await job.evidence()).files).toEqual([]);
    expect((await job.cancel("plan changed")).cancelled).toBe(true);
    expect((await job.release()).released).toBe(1);
    expect((await job.hold("not_done", "the gate is still open")).held).toBe(1);
    expect(calls).toHaveLength(6);
    for (const c of calls) {
      expect(c.headers["Authorization"]).toBe("Bearer lbt_abc");
      expect(c.headers["X-Lamdis-Key"]).toBeUndefined();
    }
    expect(calls[3].body).toEqual({ reason: "plan changed" });
    expect(calls[5].body).toEqual({ ground: "not_done", reason: "the gate is still open" });
  });
});

describe("with an agent key", () => {
  it("sends X-Lamdis-Key and normalises a funded post", async () => {
    const { fetch, calls } = fakeFetch({
      "POST /v1/tasks": {
        body: { job: "do-2", kind: "do", board: "https://x/board", escrowed: 1500, expires: "2026-09-03T00:00:00Z", status: "https://x/v1/jobs/do-2" },
      },
    });
    const posted = await new Lamdis({ fetch, key: "lam_sk_test" }).do({ predicate: "p", instructions: "i", fee_minor: 1500 });
    expect(calls[0].headers["X-Lamdis-Key"]).toBe("lam_sk_test");
    expect(posted.escrowedMinor).toBe(1500);
    expect(posted.expiresAt).toBe("2026-09-03T00:00:00Z");
    expect(posted.token).toBeUndefined();
    expect(posted.payAt).toBeUndefined();
  });

  it("checkFeasible() and quote() both POST /v1/quote", async () => {
    const quote = { reachable: "several", feasible: true };
    const { fetch, calls } = fakeFetch({ "POST /v1/quote": { body: quote } });
    const x = new Lamdis({ fetch, key: "lam_sk_test" });
    expect(await x.checkFeasible({ kind: "do", skills: ["ladder"], lat: 1, lon: 2 })).toEqual(quote);
    expect(await x.quote({ predicate: "gutters clear" })).toEqual(quote);
    expect(calls.map((c) => `${c.method} ${new URL(c.url).pathname}`)).toEqual(["POST /v1/quote", "POST /v1/quote"]);
    expect(calls[0].body).toEqual({ kind: "do", skills: ["ladder"], lat: 1, lon: 2 });
  });

  it("board() GETs /v1/board and exposes terms", async () => {
    const board = { work: [], reviews_waiting: 0, personalized: false, terms: { fee_bp: 0, payout_threshold_minor: 2000, currency: "USD" } };
    const { fetch, calls } = fakeFetch({ "GET /v1/board": { body: board } });
    const got = await new Lamdis({ fetch }).board();
    expect(got.terms.fee_bp).toBe(0);
    expect(got.terms.payout_threshold_minor).toBe(2000);
    expect(calls[0].method).toBe("GET");
    expect(calls[0].body).toBeUndefined();
  });

  it("job(id) without a token relies on the key", async () => {
    const { fetch, calls } = fakeFetch({ "GET /v1/jobs/do-2/bids": { body: { job: "do-2", bids: [] } } });
    await new Lamdis({ fetch, key: "lam_sk_test" }).job("do-2").bids();
    expect(calls[0].headers["Authorization"]).toBeUndefined();
    expect(calls[0].headers["X-Lamdis-Key"]).toBe("lam_sk_test");
  });
});

describe("errors and options", () => {
  it("throws LamdisError carrying the exchange's own sentence and status", async () => {
    const { fetch } = fakeFetch({
      "POST /v1/tasks": { status: 400, body: { error: "a job must pay for the work it asks for" } },
    });
    const err = await new Lamdis({ fetch }).observe({ predicate: "x", fee_minor: 0 }).catch((e) => e);
    expect(err).toBeInstanceOf(LamdisError);
    expect(err.status).toBe(400);
    expect(err.message).toBe("a job must pay for the work it asks for");
    expect(err.body).toEqual({ error: "a job must pay for the work it asks for" });
  });

  it("strips a trailing slash from baseUrl", async () => {
    const { fetch, calls } = fakeFetch({ "GET /v1/board": { body: { work: [] } } });
    await new Lamdis({ fetch, baseUrl: "http://localhost:8421/" }).board();
    expect(calls[0].url).toBe("http://localhost:8421/v1/board");
  });
});
