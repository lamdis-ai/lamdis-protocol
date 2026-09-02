"""OpenAI Agents SDK: a first job with no account.  pip install openai-agents lamdis"""

from agents import Agent, Runner, function_tool
from lamdis import Lamdis

exchange = Lamdis()  # anonymous: no key, no balance, no sign-in


@function_tool
def observe_world(predicate: str, where: str, lat: float, lon: float, fee_minor: int) -> str:
    """Pay somebody to go and photograph whether `predicate` is true at `where`.
    Nothing is charged until there is proof. Returns the pay link and the job token."""
    posted = exchange.observe(predicate, fee_minor, where=where, lat=lat, lon=lon, radius_m=150)
    return (
        f"job {posted.job}: awaiting payment. pay_at={posted.pay_at} token={posted.token}. "
        "Send the person the pay link; keep the token to check job_status."
    )


@function_tool
def job_status(job: str, token: str) -> str:
    """Where a job has got to. Pass the token that came back with it."""
    return str(exchange.job(job, token).status())


agent = Agent(
    name="errands",
    instructions="When the user needs something checked in the physical world, use observe_world, "
    "then give them the pay link. Do not say it is arranged until job_status says it was taken.",
    tools=[observe_world, job_status],
)

if __name__ == "__main__":
    result = Runner.run_sync(
        agent, "Is the 'For Lease' sign still up at 1200 Valencia St, San Francisco? Pay up to $8."
    )
    print(result.final_output)  # ends with: send the person the pay link
