/**
 * Vercel AI SDK tools for the Lamdis Exchange: pay people for physical work.
 *
 *   import { lamdisTools } from "@lamdis/ai-sdk-tools";
 *   generateText({ model, tools: lamdisTools, ... })
 *
 * Three tools: lamdis_check_feasible, lamdis_run_job (sandbox by default),
 * lamdis_job_status. No account, key or card is needed for any of them; a
 * sandbox job costs nothing and involves nobody. Every tool returns the
 * exchange's JSON exactly as it came back; an HTTP error becomes
 * { error, http_status } rather than a throw, because a model can act on a
 * sentence and cannot act on a stack trace.
 */
import { tool } from "ai";
import { z } from "zod";

export const DEFAULT_BASE_URL = "https://exchange.lamdis.ai";

export interface LamdisOptions {
  /** Another exchange, e.g. a private deployment. Defaults to LAMDIS_BASE_URL or exchange.lamdis.ai. */
  baseUrl?: string;
  /** A fetch to use instead of the global one. */
  fetch?: typeof fetch;
}

/** The shape every tool returns: the exchange's JSON, or an error with the HTTP status. */
export type LamdisResult = Record<string, unknown> & { error?: string; http_status?: number };

function resolveBase(opts: LamdisOptions): string {
  const env: string | undefined = (globalThis as any).process?.env?.LAMDIS_BASE_URL;
  return (opts.baseUrl ?? env ?? DEFAULT_BASE_URL).replace(/\/+$/, "");
}

async function request(
  opts: LamdisOptions,
  method: "GET" | "POST",
  path: string,
  body?: unknown,
  token?: string,
): Promise<LamdisResult> {
  const f = opts.fetch ?? fetch;
  const headers: Record<string, string> = {
    "content-type": "application/json",
    "user-agent": "lamdis-ai-sdk-tools/0.1",
    "x-lamdis-posted-by": "agent",
  };
  if (token) headers.authorization = `Bearer ${token}`;
  let r: Response;
  try {
    r = await f(resolveBase(opts) + path, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch (e) {
    return { error: `could not reach the exchange: ${(e as Error).message}`, http_status: 0 };
  }
  const text = await r.text();
  let out: LamdisResult;
  try {
    const parsed = JSON.parse(text);
    out = parsed && typeof parsed === "object" ? parsed : { error: String(parsed) };
  } catch {
    out = { error: text.trim() || r.statusText };
  }
  if (r.status >= 400) {
    out.error ??= r.statusText;
    out.http_status = r.status;
  }
  return out;
}

// ---- the three calls, usable without the AI SDK ----------------------------

export interface QuoteInput {
  predicate: string;
  lat: number;
  lon: number;
  kind?: "observe" | "do";
  skills?: string[];
  sandbox?: boolean;
}

/** POST /v1/quote: can anybody take this, would it be refused, what has it cost. */
export function quote(input: QuoteInput, opts: LamdisOptions = {}): Promise<LamdisResult> {
  const body: Record<string, unknown> = {
    kind: input.kind ?? "observe", predicate: input.predicate, lat: input.lat, lon: input.lon,
  };
  if (input.skills?.length) body.skills = input.skills;
  if (input.sandbox) body.sandbox = true;
  return request(opts, "POST", "/v1/quote", body);
}

export interface PostTaskInput {
  predicate: string;
  lat: number;
  lon: number;
  fee_minor: number;
  kind?: "observe" | "do";
  radius_m?: number;
  where?: string;
  area?: string;
  instructions?: string;
  deliverable?: string;
  skills?: string[];
  attempt_minor?: number;
  /** Default true. */
  sandbox?: boolean;
}

/** POST /v1/tasks with no credential: sandbox, or a real job that comes back with pay_at. */
export function postTask(input: PostTaskInput, opts: LamdisOptions = {}): Promise<LamdisResult> {
  const body: Record<string, unknown> = {
    kind: input.kind ?? "observe", predicate: input.predicate, lat: input.lat, lon: input.lon,
    radius_m: input.radius_m ?? 150, fee_minor: input.fee_minor,
  };
  for (const k of ["where", "area", "instructions", "deliverable", "skills", "attempt_minor"] as const) {
    const v = input[k];
    if (v !== undefined && v !== null && v !== "" && !(Array.isArray(v) && v.length === 0)) body[k] = v;
  }
  if (input.sandbox ?? true) body.sandbox = true;
  return request(opts, "POST", "/v1/tasks", body);
}

/** GET /v1/jobs/{job} with the lbt_ token that came back when it was posted. */
export function jobStatus(job: string, token: string, opts: LamdisOptions = {}): Promise<LamdisResult> {
  return request(opts, "GET", `/v1/jobs/${encodeURIComponent(job)}`, undefined, token);
}

/** GET /v1/jobs/{job}/receipt: the signed receipt once the job has settled. */
export function jobReceipt(job: string, token: string, opts: LamdisOptions = {}): Promise<LamdisResult> {
  return request(opts, "GET", `/v1/jobs/${encodeURIComponent(job)}/receipt`, undefined, token);
}

// ---- the AI SDK tools --------------------------------------------------------

export const checkFeasibleSchema = z.object({
  predicate: z.string().describe("What should be true, or be made true, at the place."),
  lat: z.number().describe("Latitude of the place."),
  lon: z.number().describe("Longitude of the place."),
  kind: z.enum(["observe", "do"]).default("observe")
    .describe('"observe" to find out whether predicate is true there, "do" to make it true.'),
  skills: z.array(z.string()).optional().describe('Qualifications required, e.g. ["vehicle"]. Usually none.'),
  sandbox: z.boolean().default(false)
    .describe("Ask the sandbox instead: feasible everywhere, real nowhere."),
});

export const runJobSchema = z.object({
  predicate: z.string().describe("What should be true when the job is finished."),
  lat: z.number().describe("Latitude of the place."),
  lon: z.number().describe("Longitude of the place."),
  fee_minor: z.number().int().positive()
    .describe("What finishing pays, in cents. Paid on verified evidence either way."),
  kind: z.enum(["observe", "do"]).default("observe").describe('"observe" (default) or "do".'),
  radius_m: z.number().int().positive().default(150)
    .describe("How close to (lat, lon) the evidence must be captured."),
  where: z.string().optional().describe("The street address. Shown only to whoever takes the job."),
  instructions: z.string().optional().describe("For a do-job, what the worker should actually do."),
  deliverable: z.string().optional().describe("What proof is expected back."),
  sandbox: z.boolean().default(true).describe(
    "Default true: costs nothing, needs no account, the job walks the real state machine " +
    "against a simulated operator in about ten seconds, nobody is dispatched and no money " +
    "moves. false posts a real job."),
});

export const jobStatusSchema = z.object({
  job: z.string().describe("The job id that came back from lamdis_run_job."),
  token: z.string().describe("The lbt_ token that came back with it."),
});

/** Build the three tools against a particular exchange. `lamdisTools` is this with defaults. */
export function createLamdisTools(opts: LamdisOptions = {}) {
  return {
    lamdis_check_feasible: tool({
      description:
        "Ask the Lamdis Exchange whether anybody near (lat, lon) could do a piece of physical " +
        "work, before promising anyone anything. Free, no account, holds no money. Returns " +
        "reachable (none, a few, several, plenty, or simulated for the sandbox), feasible, why, " +
        "optional advice, and settled_here (what this shape of work has actually been paid, when " +
        "there is enough history). If feasible is false, do not tell the person the work is arranged.",
      inputSchema: checkFeasibleSchema,
      execute: async (input) => quote(input, opts),
    }),
    lamdis_run_job: tool({
      description:
        "Post a job on the Lamdis Exchange: pay a person to check (kind=observe) or to do (kind=do) " +
        "something at a place. Nothing is charged until there is verified evidence. With sandbox=true " +
        "(the default) it returns job, token, status (a URL), sandbox=true and a note saying nothing " +
        "was real; poll lamdis_job_status with the token. With sandbox=false it posts a real job and " +
        "returns pay_at, token, amount_minor and expires_at: give the person the pay_at link, keep the " +
        "token, and do not say the work is arranged until lamdis_job_status shows it taken.",
      inputSchema: runJobSchema,
      execute: async (input) => postTask(input, opts),
    }),
    lamdis_job_status: tool({
      description:
        "Where a Lamdis job stands. Returns taken, submissions and results (each with verified and " +
        "why) for a listed job, or status awaiting_payment with pay_at for a real job whose card has " +
        "not been authorised yet. A sandbox job says sandbox=true and carries a note that nothing was real.",
      inputSchema: jobStatusSchema,
      execute: async ({ job, token }) => jobStatus(job, token, opts),
    }),
  };
}

/** The three tools against exchange.lamdis.ai. Spread into `tools:` or pass as-is. */
export const lamdisTools = createLamdisTools();

export const lamdisCheckFeasible = lamdisTools.lamdis_check_feasible;
export const lamdisRunJob = lamdisTools.lamdis_run_job;
export const lamdisJobStatus = lamdisTools.lamdis_job_status;
