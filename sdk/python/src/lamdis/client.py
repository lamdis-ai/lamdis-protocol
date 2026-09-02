"""The HTTP client. Every field here is one the exchange reads or writes; the
full surface is spec/openapi.yaml at the repository root."""

from __future__ import annotations

import json
import urllib.error
import urllib.request
from dataclasses import dataclass, field
from typing import Any, Dict, Optional

DEFAULT_BASE_URL = "https://exchange.lamdis.ai"

HOLD_GROUNDS = ("not_done", "wrong_place", "fabricated", "damage", "unsafe")


class LamdisError(Exception):
    """A refusal from the exchange. ``str(err)`` is the exchange's own sentence."""

    def __init__(self, status: int, body: Any):
        self.status = status
        self.body = body
        message = None
        if isinstance(body, dict) and isinstance(body.get("error"), str):
            message = body["error"]
        super().__init__(message or f"the exchange answered {status}")


@dataclass
class Posted:
    """What comes back from posting a job, normalised across the two funding paths."""

    job: str
    kind: str
    #: ``awaiting_payment`` for an anonymous post; the status URL for a funded one.
    status: str
    #: Anonymous only: the pay link to send the person. Nothing is charged until proof.
    pay_at: Optional[str] = None
    #: Anonymous only: the ``lbt_…`` token that follows this one job. Keep it.
    token: Optional[str] = None
    #: Anonymous only: a page a person can open to follow the job.
    watch: Optional[str] = None
    #: Anonymous only: the ceiling the card is authorised for.
    amount_minor: Optional[int] = None
    currency: Optional[str] = None
    #: Anonymous: when the unpaid job is dropped. Funded: when the job expires.
    expires_at: Optional[str] = None
    #: Funded only: the ceiling held from the balance.
    escrowed_minor: Optional[int] = None
    #: Present when the predicate repeats the street address.
    warning: Optional[str] = None
    #: The response exactly as the exchange sent it.
    raw: Dict[str, Any] = field(default_factory=dict)

    @classmethod
    def from_raw(cls, raw: Dict[str, Any]) -> "Posted":
        def s(k: str) -> Optional[str]:
            v = raw.get(k)
            return v if isinstance(v, str) else None

        def n(k: str) -> Optional[int]:
            v = raw.get(k)
            return v if isinstance(v, int) and not isinstance(v, bool) else None

        return cls(
            job=str(raw.get("job", "")),
            kind=s("kind") or "observe",
            status=s("status") or "",
            pay_at=s("pay_at"),
            token=s("token"),
            watch=s("watch"),
            amount_minor=n("amount_minor"),
            currency=s("currency"),
            expires_at=s("expires_at") or s("expires"),
            escrowed_minor=n("escrowed"),
            warning=s("warning"),
            raw=raw,
        )


class Lamdis:
    """A client bound to one exchange and, optionally, one agent key."""

    def __init__(
        self,
        base_url: str = DEFAULT_BASE_URL,
        key: Optional[str] = None,
        timeout: float = 30.0,
        opener: Optional[urllib.request.OpenerDirector] = None,
    ):
        self.base_url = base_url.rstrip("/")
        self.key = key
        self.timeout = timeout
        self._opener = opener or urllib.request.build_opener()

    # -- asking --------------------------------------------------------------

    def check_feasible(self, **req: Any) -> Dict[str, Any]:
        """Is anybody reachable, would this be refused, what has it settled at.

        Holds nothing. Requires a key: the exchange answers 401 to an
        anonymous quote.
        """
        return self.quote(**req)

    def quote(self, **req: Any) -> Dict[str, Any]:
        """The same call as ``check_feasible``, under the endpoint's own name."""
        return self.request("POST", "/v1/quote", req)

    # -- posting -------------------------------------------------------------

    def observe(self, predicate: str, fee_minor: int, **req: Any) -> Posted:
        """Find out whether something is true in the world. Paid either way."""
        return self.post(kind="observe", predicate=predicate, fee_minor=fee_minor, **req)

    def do(self, predicate: str, instructions: str, fee_minor: int, **req: Any) -> Posted:
        """Have something in the world made true, with proof it happened."""
        return self.post(
            kind="do", predicate=predicate, instructions=instructions, fee_minor=fee_minor, **req
        )

    def post(self, **req: Any) -> Posted:
        """``POST /v1/tasks`` with the request exactly as given (wire field names)."""
        return Posted.from_raw(self.request("POST", "/v1/tasks", req))

    # -- following -----------------------------------------------------------

    def job(self, job_id: str, token: Optional[str] = None) -> "Job":
        """A handle on one job. Pass the token from an anonymous post."""
        return Job(self, job_id, token)

    def board(self) -> Dict[str, Any]:
        """Open work, as a stranger sees it."""
        return self.request("GET", "/v1/board")

    # -- transport -----------------------------------------------------------

    def request(
        self,
        method: str,
        path: str,
        body: Optional[Any] = None,
        token: Optional[str] = None,
    ) -> Any:
        data = None
        headers = {
            "Accept": "application/json",
            # Software wrote this; workers are told.
            "X-Lamdis-Posted-By": "agent",
        }
        if body is not None:
            data = json.dumps(body).encode("utf-8")
            headers["Content-Type"] = "application/json"
        if self.key:
            headers["X-Lamdis-Key"] = self.key
        if token:
            headers["Authorization"] = "Bearer " + token
        req = urllib.request.Request(
            self.base_url + path, data=data, method=method, headers=headers
        )
        try:
            with self._opener.open(req, timeout=self.timeout) as resp:
                return _decode(resp.read())
        except urllib.error.HTTPError as e:
            raise LamdisError(e.code, _decode(e.read())) from None


def _decode(raw: bytes) -> Any:
    if not raw:
        return None
    try:
        return json.loads(raw.decode("utf-8"))
    except (ValueError, UnicodeDecodeError):
        return raw.decode("utf-8", "replace")


class Job:
    """One job, addressed by id and (for an anonymous post) its token."""

    def __init__(self, client: Lamdis, job_id: str, token: Optional[str] = None):
        self.client = client
        self.id = job_id
        self.token = token

    def status(self) -> Dict[str, Any]:
        """Where it stands: pending payment, taken, submitted, checked, paid."""
        return self._call("GET", "")

    def receipt(self) -> Dict[str, Any]:
        """The signed receipt. 409 until something has been submitted."""
        return self._call("GET", "/receipt")

    def evidence(self) -> Dict[str, Any]:
        """The files somebody brought back, each with a ``view`` URL."""
        return self._call("GET", "/evidence")

    def cancel(self, reason: Optional[str] = None) -> Dict[str, Any]:
        """Withdraw a job nobody has taken yet and release its escrow."""
        return self._call("POST", "/cancel", {} if reason is None else {"reason": reason})

    def release(self) -> Dict[str, Any]:
        """The work is good: pay them now rather than after the 24-hour window."""
        return self._call("POST", "/release", {})

    def hold(self, ground: str, reason: str) -> Dict[str, Any]:
        """Something is wrong: name a ground and say why. A panel decides within 7 days."""
        if ground not in HOLD_GROUNDS:
            raise ValueError(f"ground must be one of {HOLD_GROUNDS}")
        return self._call("POST", "/hold", {"ground": ground, "reason": reason})

    def bids(self) -> Dict[str, Any]:
        """Offers on an open (bids) job."""
        return self._call("GET", "/bids")

    def award(self, bid: str) -> Dict[str, Any]:
        """Accept one offer; the amount becomes the price."""
        return self._call("POST", "/award", {"bid": bid})

    def _call(self, method: str, suffix: str, body: Optional[Any] = None) -> Any:
        return self.client.request(
            method, "/v1/jobs/" + urllib.request.quote(self.id, safe="") + suffix, body, self.token
        )
