"""OpenAI Agents SDK tools for the Lamdis Exchange: pay people for physical work.

    from agents import Agent
    from lamdis_openai_agents import lamdis_tools
    agent = Agent(name="errands", tools=lamdis_tools)

Three function tools: lamdis_check_feasible, lamdis_run_job (sandbox by
default), lamdis_job_status. Each returns the exchange's JSON as a string.
No account, key or card is needed; a sandbox job costs nothing and involves nobody.
"""

from __future__ import annotations

import json
from typing import Optional

from agents import function_tool

from . import _client

__all__ = ["lamdis_check_feasible", "lamdis_run_job", "lamdis_job_status", "lamdis_tools",
           "job_receipt", "DEFAULT_BASE_URL"]

DEFAULT_BASE_URL = _client.DEFAULT_BASE_URL
job_receipt = _client.job_receipt


@function_tool
def lamdis_check_feasible(
    predicate: str,
    lat: float,
    lon: float,
    kind: str = "observe",
    skills: Optional[list[str]] = None,
    sandbox: bool = False,
) -> str:
    """Ask the Lamdis Exchange whether anybody near (lat, lon) could do a piece of
    physical work, before promising anyone anything. Free, no account, holds no money.
    If feasible is false, do not tell the person the work is arranged.

    Args:
        predicate: What should be true, or be made true, at the place.
        lat: Latitude of the place.
        lon: Longitude of the place.
        kind: "observe" to find out whether predicate is true there, "do" to make it true.
        skills: Qualifications required, e.g. ["vehicle", "ladder"]. Usually none.
        sandbox: Ask the sandbox instead: feasible everywhere, real nowhere. Default false.

    Returns: JSON with reachable ("none", "a few", "several", "plenty", or "simulated"),
    feasible, why, optional advice, and settled_here (what this shape of work has
    actually been paid, when there is enough history).
    """
    return json.dumps(_client.quote(predicate, lat, lon, kind=kind, skills=skills, sandbox=sandbox))


@function_tool
def lamdis_run_job(
    predicate: str,
    lat: float,
    lon: float,
    fee_minor: int,
    kind: str = "observe",
    radius_m: int = 150,
    where: Optional[str] = None,
    instructions: Optional[str] = None,
    deliverable: Optional[str] = None,
    sandbox: bool = True,
) -> str:
    """Post a job on the Lamdis Exchange: pay a person to check (kind="observe") or
    to do (kind="do") something at a place. Nothing is charged until there is
    verified evidence.

    Args:
        predicate: What should be true when the job is finished.
        lat: Latitude of the place.
        lon: Longitude of the place.
        fee_minor: What finishing pays, in cents. Paid on verified evidence either way.
        kind: "observe" (default) or "do".
        radius_m: How close to (lat, lon) the evidence must be captured. Default 150.
        where: The street address. Shown only to whoever takes the job.
        instructions: For a do-job, what the worker should actually do.
        deliverable: What proof is expected back.
        sandbox: Default true: costs nothing, needs no account, the job walks the real
            state machine against a simulated operator in about ten seconds, nobody is
            dispatched and no money moves. false posts a real job.

    Returns: for a sandbox job, JSON with job, token, status (a URL), sandbox=true and
    a note saying nothing was real; poll lamdis_job_status with the token. For a real
    job, JSON with pay_at, token, amount_minor and expires_at: give the person the
    pay_at link, keep the token, and do not say the work is arranged until
    lamdis_job_status shows it taken.
    """
    return json.dumps(_client.post_task(
        predicate, lat, lon, fee_minor, kind=kind, radius_m=radius_m, where=where,
        instructions=instructions, deliverable=deliverable, sandbox=sandbox,
    ))


@function_tool
def lamdis_job_status(job: str, token: str) -> str:
    """Where a Lamdis job stands.

    Args:
        job: The job id that came back from lamdis_run_job.
        token: The lbt_ token that came back with it.

    Returns: JSON with taken, submissions and results (each with verified and why)
    for a listed job, or status "awaiting_payment" with pay_at for a real job whose
    card has not been authorised yet. A sandbox job says sandbox=true and carries a
    note that nothing was real.
    """
    return json.dumps(_client.job_status(job, token))


lamdis_tools = [lamdis_check_feasible, lamdis_run_job, lamdis_job_status]
