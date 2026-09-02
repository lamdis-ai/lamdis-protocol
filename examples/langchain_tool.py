"""LangChain: a first job with no account.  pip install langchain-core lamdis"""

from langchain_core.tools import tool
from lamdis import Lamdis

exchange = Lamdis()  # anonymous: no key, no balance, no sign-in


@tool
def observe_world(predicate: str, where: str, lat: float, lon: float, fee_minor: int) -> dict:
    """Pay somebody to go and photograph whether `predicate` is true at `where`.
    fee_minor is in cents and is paid for honest evidence whichever way the answer
    turns out. Nothing is charged until there is proof. Returns pay_at and token."""
    posted = exchange.observe(predicate, fee_minor, where=where, lat=lat, lon=lon, radius_m=150)
    return {"job": posted.job, "status": posted.status, "pay_at": posted.pay_at, "token": posted.token}


@tool
def job_status(job: str, token: str) -> dict:
    """Where a job has got to. Pass the token that came back when it was posted."""
    return exchange.job(job, token).status()


tools = [observe_world, job_status]

if __name__ == "__main__":
    # Bind `tools` to any chat model that supports tool calling, e.g.
    #   llm.bind_tools(tools)  or  create_react_agent(llm, tools)
    out = observe_world.invoke(
        {"predicate": "The 'For Lease' sign is still up on the corner unit",
         "where": "1200 Valencia St, San Francisco", "lat": 37.7527, "lon": -122.4207, "fee_minor": 800}
    )
    print(out["pay_at"])  # send the person the pay link
