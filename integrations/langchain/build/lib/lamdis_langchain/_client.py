"""The three calls every integration makes, over plain HTTP.

Kept to one file with no dependency but httpx so the package installs from a
GitHub subdirectory with nothing else. Every function returns the exchange's
JSON exactly as it came back; an HTTP error becomes {"error": ..., "http_status": n}
rather than an exception, because an agent can act on a sentence and cannot
act on a traceback.
"""

from __future__ import annotations

import os
from typing import Any, Optional

import httpx

DEFAULT_BASE_URL = "https://exchange.lamdis.ai"
USER_AGENT = "lamdis-langchain/0.1"


def base_url() -> str:
    return os.environ.get("LAMDIS_BASE_URL", DEFAULT_BASE_URL).rstrip("/")


def _request(method: str, path: str, *, json: Any = None, token: Optional[str] = None) -> dict:
    headers = {"user-agent": USER_AGENT, "x-lamdis-posted-by": "agent"}
    if token:
        headers["authorization"] = f"Bearer {token}"
    try:
        r = httpx.request(method, base_url() + path, json=json, headers=headers, timeout=30.0)
    except httpx.HTTPError as e:
        return {"error": f"could not reach the exchange: {e}", "http_status": 0}
    try:
        body = r.json()
    except ValueError:
        body = {"error": r.text.strip() or r.reason_phrase}
    if r.status_code >= 400:
        if not isinstance(body, dict):
            body = {"error": str(body)}
        body.setdefault("error", r.reason_phrase)
        body["http_status"] = r.status_code
    return body


def quote(
    predicate: str,
    lat: float,
    lon: float,
    kind: str = "observe",
    skills: Optional[list[str]] = None,
    sandbox: bool = False,
) -> dict:
    """POST /v1/quote: can anybody take this, would it be refused, what has it cost."""
    body: dict[str, Any] = {"kind": kind, "predicate": predicate, "lat": lat, "lon": lon}
    if skills:
        body["skills"] = skills
    if sandbox:
        body["sandbox"] = True
    return _request("POST", "/v1/quote", json=body)


def post_task(
    predicate: str,
    lat: float,
    lon: float,
    fee_minor: int,
    kind: str = "observe",
    radius_m: int = 150,
    where: Optional[str] = None,
    area: Optional[str] = None,
    instructions: Optional[str] = None,
    deliverable: Optional[str] = None,
    skills: Optional[list[str]] = None,
    attempt_minor: Optional[int] = None,
    sandbox: bool = True,
) -> dict:
    """POST /v1/tasks with no credential: sandbox, or a real job that comes back with pay_at."""
    body: dict[str, Any] = {
        "kind": kind, "predicate": predicate, "lat": lat, "lon": lon,
        "radius_m": radius_m, "fee_minor": fee_minor,
    }
    for k, v in (("where", where), ("area", area), ("instructions", instructions),
                 ("deliverable", deliverable), ("skills", skills), ("attempt_minor", attempt_minor)):
        if v:
            body[k] = v
    if sandbox:
        body["sandbox"] = True
    return _request("POST", "/v1/tasks", json=body)


def job_status(job: str, token: str) -> dict:
    """GET /v1/jobs/{job} with the lbt_ token that came back when it was posted."""
    return _request("GET", f"/v1/jobs/{job}", token=token)


def job_receipt(job: str, token: str) -> dict:
    """GET /v1/jobs/{job}/receipt: the signed receipt once the job has settled."""
    return _request("GET", f"/v1/jobs/{job}/receipt", token=token)
