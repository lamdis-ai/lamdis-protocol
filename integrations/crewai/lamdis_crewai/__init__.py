"""CrewAI tools for the Lamdis Exchange: pay people for physical work.

    from crewai import Agent
    from lamdis_crewai import lamdis_tools
    agent = Agent(role="errands", goal=..., backstory=..., tools=lamdis_tools)

Three BaseTool subclasses: LamdisCheckFeasible, LamdisRunJob (sandbox by
default), LamdisJobStatus; lamdis_tools holds one instance of each. Each
returns the exchange's JSON as a string. No account, key or card is needed;
a sandbox job costs nothing and involves nobody.
"""

from __future__ import annotations

import json
from typing import Optional, Type

from crewai.tools import BaseTool
from pydantic import BaseModel, Field

from . import _client

__all__ = ["LamdisCheckFeasible", "LamdisRunJob", "LamdisJobStatus",
           "lamdis_check_feasible", "lamdis_run_job", "lamdis_job_status", "lamdis_tools",
           "job_receipt", "DEFAULT_BASE_URL"]

DEFAULT_BASE_URL = _client.DEFAULT_BASE_URL
job_receipt = _client.job_receipt


class CheckFeasibleArgs(BaseModel):
    predicate: str = Field(description="What should be true, or be made true, at the place.")
    lat: float = Field(description="Latitude of the place.")
    lon: float = Field(description="Longitude of the place.")
    kind: str = Field(default="observe", description='"observe" to find out whether predicate is true there, "do" to make it true.')
    skills: Optional[list[str]] = Field(default=None, description='Qualifications required, e.g. ["vehicle"]. Usually none.')
    sandbox: bool = Field(default=False, description="Ask the sandbox instead: feasible everywhere, real nowhere.")


class LamdisCheckFeasible(BaseTool):
    name: str = "lamdis_check_feasible"
    description: str = (
        "Ask the Lamdis Exchange whether anybody near (lat, lon) could do a piece of "
        "physical work, before promising anyone anything. Free, no account, holds no "
        "money. Returns JSON with reachable (none, a few, several, plenty, or simulated "
        "for the sandbox), feasible, why, optional advice, and settled_here (what this "
        "shape of work has actually been paid, when there is enough history). If "
        "feasible is false, do not tell the person the work is arranged."
    )
    args_schema: Type[BaseModel] = CheckFeasibleArgs

    def _run(self, predicate: str, lat: float, lon: float, kind: str = "observe",
             skills: Optional[list[str]] = None, sandbox: bool = False) -> str:
        return json.dumps(_client.quote(predicate, lat, lon, kind=kind, skills=skills, sandbox=sandbox))


class RunJobArgs(BaseModel):
    predicate: str = Field(description="What should be true when the job is finished.")
    lat: float = Field(description="Latitude of the place.")
    lon: float = Field(description="Longitude of the place.")
    fee_minor: int = Field(description="What finishing pays, in cents. Paid on verified evidence either way.")
    kind: str = Field(default="observe", description='"observe" (default) or "do".')
    radius_m: int = Field(default=150, description="How close to (lat, lon) the evidence must be captured.")
    where: Optional[str] = Field(default=None, description="The street address. Shown only to whoever takes the job.")
    instructions: Optional[str] = Field(default=None, description="For a do-job, what the worker should actually do.")
    deliverable: Optional[str] = Field(default=None, description="What proof is expected back.")
    sandbox: bool = Field(default=True, description=(
        "Default true: costs nothing, needs no account, the job walks the real state "
        "machine against a simulated operator in about ten seconds, nobody is dispatched "
        "and no money moves. false posts a real job."))


class LamdisRunJob(BaseTool):
    name: str = "lamdis_run_job"
    description: str = (
        "Post a job on the Lamdis Exchange: pay a person to check (kind=observe) or to "
        "do (kind=do) something at a place. Nothing is charged until there is verified "
        "evidence. With sandbox=true (the default) it returns JSON with job, token, "
        "status (a URL), sandbox=true and a note saying nothing was real; poll "
        "lamdis_job_status with the token. With sandbox=false it posts a real job and "
        "returns pay_at, token, amount_minor and expires_at: give the person the pay_at "
        "link, keep the token, and do not say the work is arranged until "
        "lamdis_job_status shows it taken."
    )
    args_schema: Type[BaseModel] = RunJobArgs

    def _run(self, predicate: str, lat: float, lon: float, fee_minor: int, kind: str = "observe",
             radius_m: int = 150, where: Optional[str] = None, instructions: Optional[str] = None,
             deliverable: Optional[str] = None, sandbox: bool = True) -> str:
        return json.dumps(_client.post_task(
            predicate, lat, lon, fee_minor, kind=kind, radius_m=radius_m, where=where,
            instructions=instructions, deliverable=deliverable, sandbox=sandbox,
        ))


class JobStatusArgs(BaseModel):
    job: str = Field(description="The job id that came back from lamdis_run_job.")
    token: str = Field(description="The lbt_ token that came back with it.")


class LamdisJobStatus(BaseTool):
    name: str = "lamdis_job_status"
    description: str = (
        "Where a Lamdis job stands. Returns JSON with taken, submissions and results "
        "(each with verified and why) for a listed job, or status awaiting_payment with "
        "pay_at for a real job whose card has not been authorised yet. A sandbox job "
        "says sandbox=true and carries a note that nothing was real."
    )
    args_schema: Type[BaseModel] = JobStatusArgs

    def _run(self, job: str, token: str) -> str:
        return json.dumps(_client.job_status(job, token))


lamdis_check_feasible = LamdisCheckFeasible()
lamdis_run_job = LamdisRunJob()
lamdis_job_status = LamdisJobStatus()
lamdis_tools = [lamdis_check_feasible, lamdis_run_job, lamdis_job_status]
