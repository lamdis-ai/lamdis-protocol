"""LangChain tools for the Lamdis Exchange: pay people for physical work.

    from lamdis_langchain import lamdis_tools
    llm.bind_tools(lamdis_tools)

Three tools: lamdis_check_feasible, lamdis_run_job (sandbox by default),
lamdis_job_status. No account, key or card is needed for any of them; a
sandbox job costs nothing and involves nobody.
"""

from __future__ import annotations

from typing import Optional

from langchain_core.tools import tool

from . import _client

__all__ = ["lamdis_check_feasible", "lamdis_run_job", "lamdis_job_status", "lamdis_tools",
           "job_receipt", "DEFAULT_BASE_URL"]

DEFAULT_BASE_URL = _client.DEFAULT_BASE_URL
job_receipt = _client.job_receipt


@tool
def lamdis_check_feasible(
    predicate: str,
    lat: float,
    lon: float,
    kind: str = "observe",
    skills: Optional[list[str]] = None,
    sandbox: bool = False,
) -> dict:
    """Ask the Lamdis Exchange whether anybody near (lat, lon) could do a piece of
    physical work, before promising anyone anything. Free, no account, holds no money.

    kind is "observe" (find out whether predicate is true there) or "do" (make it
    true). Returns reachable ("none", "a few", "several", "plenty", or "simulated"
    for the sandbox), feasible, why, optional advice, and settled_here (what this
    shape of work has actually been paid, when there is enough history). If
    feasible is false, do not tell the person the work is arranged. Pass
    sandbox=true to ask the sandbox instead, which is feasible everywhere and real
    nowhere."""
    return _client.quote(predicate, lat, lon, kind=kind, skills=skills, sandbox=sandbox)


@tool
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
) -> dict:
    """Post a job on the Lamdis Exchange: pay a person to check (kind="observe") or
    to do (kind="do") something at a place. fee_minor is cents, paid only on
    verified evidence. where is the street address; radius_m is how close the
    evidence must be captured.

    sandbox=true (the default) costs nothing and needs no account: the job walks
    the real state machine against a simulated operator in about ten seconds,
    nobody is dispatched and no money moves. It returns job, token, status (a
    URL), sandbox=true and a note saying so; poll lamdis_job_status with the
    token. sandbox=false posts a real job and returns pay_at, token, amount_minor
    and expires_at: give the person the pay_at link, keep the token, and do not
    say the work is arranged until lamdis_job_status shows it taken."""
    return _client.post_task(
        predicate, lat, lon, fee_minor, kind=kind, radius_m=radius_m, where=where,
        instructions=instructions, deliverable=deliverable, sandbox=sandbox,
    )


@tool
def lamdis_job_status(job: str, token: str) -> dict:
    """Where a Lamdis job stands. Pass the job id and the lbt_ token that came back
    from lamdis_run_job. Returns taken, submissions and results (each with
    verified and why) for a listed job, or status "awaiting_payment" with pay_at
    for a real job whose card has not been authorised yet. A sandbox job says
    sandbox=true and carries a note that nothing was real."""
    return _client.job_status(job, token)


lamdis_tools = [lamdis_check_feasible, lamdis_run_job, lamdis_job_status]
