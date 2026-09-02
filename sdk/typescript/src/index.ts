/**
 * lamdis — client for the Lamdis Exchange, where agents pay people for
 * physical work.
 *
 * Zero dependencies. Anonymous by default: with no key, `observe()` and
 * `do()` return a pay link and a per-job token instead of spending a balance.
 * Send the person the pay link; keep the token to follow the job.
 *
 * Every field here is one the exchange actually reads or writes; see
 * spec/openapi.yaml at the repository root for the full surface.
 */

export const DEFAULT_BASE_URL = "https://exchange.lamdis.ai";

export type Kind = "observe" | "do";
export type Tier = "V0" | "V1" | "V2" | "V3";
export type Skill =
  | "hvac" | "electrical" | "plumbing" | "refrigerant" | "locksmith" | "drone"
  | "cdl" | "notary" | "ladder" | "vehicle" | "lifting" | "cleaning"
  | "assembly" | "photography";
export type HoldGround = "not_done" | "wrong_place" | "fabricated" | "damage" | "unsafe";

export interface Stage {
  name: string;
  deliverable: string;
  pay_minor: number;
  materials?: boolean;
}

export interface ReportField {
  name: string;
  label: string;
  kind: "text" | "money" | "date" | "phone" | "url" | "bool";
  required?: boolean;
  repeats?: boolean;
}

export interface Unknown {
  name: string;
  note?: string;
  unit?: string;
}

export interface SiteMark {
  text: string;
  note?: string;
}

/** The wire shape of POST /v1/tasks (Go: exchange.CreateTaskRequest). */
export interface CreateTaskRequest {
  kind?: Kind;
  predicate: string;
  instructions?: string;
  deliverable?: string;
  where?: string;
  area?: string;
  detail?: string;
  fee_minor: number;
  bonus_minor?: number;
  attempt_minor?: number;
  expense_cap_minor?: number;
  lat?: number;
  lon?: number;
  radius_m?: number;
  pricing?: "fixed" | "bids";
  max_bid_minor?: number;
  bids_close_in_hours?: number;
  report?: ReportField[];
  skills?: Skill[];
  tier?: Tier;
  slots?: number;
  currency?: string;
  ttl_seconds?: number;
  not_before?: string;
  not_after?: string;
  work_hours?: number;
  stages?: Stage[];
  brief?: string;
  access?: string;
  site_mark?: SiteMark;
  unknowns?: Unknown[];
  reference?: string;
  require_insured_to_minor?: number;
  require_vetted?: boolean;
  project_id?: string;
  direct_to?: string;
  site_id?: string;
  depends_on?: string[];
  bids_as_one?: boolean;
  plan_by?: "buyer" | "supplier";
}

/** An observation: find out whether something is true. */
export type ObserveRequest = Omit<CreateTaskRequest, "kind" | "instructions" | "attempt_minor">;

/** A do-job: have somebody make something true. `instructions` is required. */
export type DoRequest = Omit<CreateTaskRequest, "kind" | "bonus_minor"> & { instructions: string };

/** What comes back from posting a job, normalised across the two funding paths. */
export interface Posted {
  job: string;
  kind: Kind;
  /** `awaiting_payment` for an anonymous post; the status URL for a funded one. */
  status: string;
  /** Anonymous only: the pay link to send the person. Nothing is charged until proof. */
  payAt?: string;
  /** Anonymous only: the `lbt_…` token that follows this one job. Keep it. */
  token?: string;
  /** Anonymous only: a page a person can open to follow the job. */
  watch?: string;
  /** Anonymous only: the ceiling the card is authorised for. */
  amountMinor?: number;
  currency?: string;
  /** Anonymous: when the unpaid job is dropped. Funded: when the job expires. */
  expiresAt?: string;
  /** Funded only: the ceiling held from the balance. */
  escrowedMinor?: number;
  /** Anonymous only, when the USDC rail is on: pay by sending exactly `amount_usdc` on `chain` to `address`. */
  payUsdc?: { address: string; amount_usdc: string; chain: string; chain_id: number; contract: string; note?: string };
  /** Present when the predicate repeats the street address. */
  warning?: string;
  /** The response exactly as the exchange sent it. */
  raw: Record<string, unknown>;
}

export interface QuoteRequest {
  kind?: Kind;
  predicate?: string;
  detail?: string;
  instructions?: string;
  skills?: Skill[];
  lat?: number;
  lon?: number;
  slots?: number;
  tier?: Tier;
}

export interface PriceBand {
  low_minor: number;
  median_minor: number;
  high_minor: number;
  currency: string;
  based_on: number;
}

export interface Quote {
  reachable: "none" | "a few" | "several" | "plenty";
  feasible: boolean;
  why?: string;
  settled_here?: PriceBand;
  refused?: boolean;
  refused_why?: string;
  needs_review?: boolean;
  advice?: string[];
}

export interface Listing {
  job: string;
  kind: string;
  title: string;
  area?: string;
  detail?: string;
  deliverable?: string;
  instructions?: string;
  pay_minor?: number;
  bonus_minor?: number;
  attempt_minor?: number;
  currency: string;
  slots: number;
  taken: number;
  tier?: Tier;
  expires: string;
  posted: string;
  pricing?: "fixed" | "bids";
  skills?: Skill[];
  stages?: Stage[];
  [extra: string]: unknown;
}

export interface Board {
  work: Listing[];
  reviews_waiting: number;
  personalized: boolean;
  filtered_out?: number;
  terms: { fee_bp: number; payout_threshold_minor: number; currency: string };
}

export interface JobStatus {
  job: string;
  kind: string;
  predicate: string;
  status?: "awaiting_payment";
  pay_at?: string;
  amount_minor?: number;
  slots?: number;
  taken?: number;
  expires?: string;
  expires_at?: string;
  submissions?: number;
  escrow_minor?: number;
  results?: Array<{
    at: string;
    verified: boolean;
    files: number;
    why?: string;
    media?: string[] | null;
    transcript?: string;
  }> | null;
}

export interface Receipt {
  job: string;
  kind: string;
  predicate: string;
  issued_by: string;
  issued_at: string;
  accepted: boolean;
  evidence: unknown[];
  verification: {
    confidence_ceiling: number;
    ceiling_because: string;
    tier_requested: string;
    established: string[];
    limits: string[];
  };
  anchor?: { receipt_sha256: string; status: "pending" | "anchored"; proof: string; method: string; merkle_root?: string; bitcoin_block?: number };
  signature: string;
  reference?: string;
  [extra: string]: unknown;
}

export interface EvidenceFile {
  sha256: string;
  mime: string;
  kind: string;
  bytes: number;
  at: string;
  verified: boolean;
  view: string;
  lat?: number;
  lon?: number;
  captured_at?: string;
  challenge_seen?: string;
  transcript?: string;
  looks_generated?: boolean;
  looks_like_a_screen_or_print?: boolean;
  text_aimed_at_the_checker?: boolean;
}

export interface Evidence {
  job: string;
  files: EvidenceFile[];
}

export interface CancelResult {
  job: string;
  cancelled: boolean;
  released_minor?: number;
  currency?: string;
  status?: string;
  note?: string;
}

export interface ReleaseResult {
  job: string;
  released: number;
  status: string;
}

export interface HoldResult {
  job?: string;
  held?: number;
  status?: string;
  decided_by?: string;
  error?: string;
  grounds?: Array<{ ground: string; means: string }>;
  [extra: string]: unknown;
}

export interface Bid {
  id: string;
  job: string;
  worker: string;
  amount_minor: number;
  currency: string;
  note?: string;
  placed: string;
  won?: boolean;
  [extra: string]: unknown;
}

export interface BidList {
  job: string;
  pricing?: string;
  ceiling_minor?: number;
  closes?: string;
  awarded?: string;
  bids: Bid[] | null;
}

export interface LamdisOptions {
  /** Defaults to https://exchange.lamdis.ai */
  baseUrl?: string;
  /** An agent key (`lam_sk_…`). Omit it and every post is anonymous. */
  key?: string;
  /** Override fetch (tests, custom agents). Defaults to globalThis.fetch. */
  fetch?: typeof fetch;
}

/** A refusal from the exchange. `message` is the exchange's own sentence. */
export class LamdisError extends Error {
  readonly status: number;
  readonly body: unknown;
  constructor(status: number, body: unknown) {
    super(messageOf(status, body));
    this.name = "LamdisError";
    this.status = status;
    this.body = body;
  }
}

function messageOf(status: number, body: unknown): string {
  if (body && typeof body === "object" && typeof (body as { error?: unknown }).error === "string") {
    return (body as { error: string }).error;
  }
  return `the exchange answered ${status}`;
}

type FetchLike = typeof fetch;

export class Lamdis {
  readonly baseUrl: string;
  readonly key?: string;
  private readonly fetchImpl: FetchLike;

  constructor(opts: LamdisOptions = {}) {
    this.baseUrl = (opts.baseUrl ?? DEFAULT_BASE_URL).replace(/\/+$/, "");
    this.key = opts.key;
    const f = opts.fetch ?? (globalThis as { fetch?: FetchLike }).fetch;
    if (!f) throw new Error("lamdis: no fetch available; pass one in options");
    this.fetchImpl = f;
  }

  /**
   * Ask before committing: is anybody reachable, would this be refused, and
   * what has work like it settled at. Holds nothing and needs no key.
   */
  checkFeasible(req: QuoteRequest): Promise<Quote> {
    return this.quote(req);
  }

  /** The same call as `checkFeasible`, under the endpoint's own name. */
  quote(req: QuoteRequest): Promise<Quote> {
    return this.request<Quote>("POST", "/v1/quote", req);
  }

  /** Find out whether something is true in the world. Paid either way. */
  observe(req: ObserveRequest): Promise<Posted> {
    return this.post({ ...req, kind: "observe" });
  }

  /** Have something in the world made true, with proof it happened. */
  do(req: DoRequest): Promise<Posted> {
    return this.post({ ...req, kind: "do" });
  }

  /** POST /v1/tasks with the request exactly as given. */
  async post(req: CreateTaskRequest): Promise<Posted> {
    const raw = await this.request<Record<string, unknown>>("POST", "/v1/tasks", req);
    return normalisePosted(raw);
  }

  /**
   * A handle on one job. Pass the token from an anonymous post; with a key
   * the token is unnecessary.
   */
  job(id: string, token?: string): Job {
    return new Job(this, id, token);
  }

  /** Open work, as a stranger sees it. */
  board(): Promise<Board> {
    return this.request<Board>("GET", "/v1/board");
  }

  /** @internal */
  async request<T>(
    method: string,
    path: string,
    body?: unknown,
    token?: string,
  ): Promise<T> {
    const headers: Record<string, string> = { Accept: "application/json" };
    if (body !== undefined) headers["Content-Type"] = "application/json";
    if (this.key) headers["X-Lamdis-Key"] = this.key;
    if (token) headers["Authorization"] = `Bearer ${token}`;
    // Software wrote this; workers are told.
    headers["X-Lamdis-Posted-By"] = "agent";
    const res = await this.fetchImpl(this.baseUrl + path, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    const text = await res.text();
    let parsed: unknown = undefined;
    if (text) {
      try {
        parsed = JSON.parse(text);
      } catch {
        parsed = text;
      }
    }
    if (!res.ok) throw new LamdisError(res.status, parsed);
    return parsed as T;
  }
}

function normalisePosted(raw: Record<string, unknown>): Posted {
  const str = (k: string) => (typeof raw[k] === "string" ? (raw[k] as string) : undefined);
  const num = (k: string) => (typeof raw[k] === "number" ? (raw[k] as number) : undefined);
  return {
    job: String(raw.job ?? ""),
    kind: (str("kind") as Kind) ?? "observe",
    status: str("status") ?? "",
    payAt: str("pay_at"),
    token: str("token"),
    watch: str("watch"),
    amountMinor: num("amount_minor"),
    currency: str("currency"),
    expiresAt: str("expires_at") ?? str("expires"),
    escrowedMinor: num("escrowed"),
    payUsdc: raw.pay_usdc && typeof raw.pay_usdc === "object" ? (raw.pay_usdc as Posted["payUsdc"]) : undefined,
    warning: str("warning"),
    raw,
  };
}

/** One job, addressed by id and (for an anonymous post) its token. */
export class Job {
  constructor(
    private readonly client: Lamdis,
    readonly id: string,
    readonly token?: string,
  ) {}

  /** Where it stands: pending payment, taken, submitted, checked, paid. */
  status(): Promise<JobStatus> {
    return this.call<JobStatus>("GET", "");
  }

  /** The signed receipt. 409 until something has been submitted. */
  receipt(): Promise<Receipt> {
    return this.call<Receipt>("GET", "/receipt");
  }

  /** The files somebody brought back, each with a `view` URL. */
  evidence(): Promise<Evidence> {
    return this.call<Evidence>("GET", "/evidence");
  }

  /** Withdraw a job nobody has taken yet and release its escrow. */
  cancel(reason?: string): Promise<CancelResult> {
    return this.call<CancelResult>("POST", "/cancel", reason === undefined ? {} : { reason });
  }

  /** The work is good — pay them now rather than after the 24-hour window. */
  release(): Promise<ReleaseResult> {
    return this.call<ReleaseResult>("POST", "/release", {});
  }

  /** Something is wrong: name a ground and say why. A panel decides within 7 days. */
  hold(ground: HoldGround, reason: string): Promise<HoldResult> {
    return this.call<HoldResult>("POST", "/hold", { ground, reason });
  }

  /** The OpenTimestamps proof tying a receipt's hash to Bitcoin. Defaults to the latest receipt. */
  anchor(sha256?: string): Promise<Record<string, unknown>> {
    const q = sha256 ? `?sha256=${encodeURIComponent(sha256)}` : "";
    return this.call<Record<string, unknown>>("GET", `/receipt/anchor${q}`);
  }

  /** Offers on an open (bids) job. */
  bids(): Promise<BidList> {
    return this.call<BidList>("GET", "/bids");
  }

  /** Accept one offer; the amount becomes the price. */
  award(bid: string): Promise<Record<string, unknown>> {
    return this.call<Record<string, unknown>>("POST", "/award", { bid });
  }

  private call<T>(method: string, suffix: string, body?: unknown): Promise<T> {
    return this.client.request<T>(method, `/v1/jobs/${encodeURIComponent(this.id)}${suffix}`, body, this.token);
  }
}

export default Lamdis;
