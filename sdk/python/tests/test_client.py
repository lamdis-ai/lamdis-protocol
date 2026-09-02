"""Tests against a fake exchange on localhost, so nothing here needs network."""

import json
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import pytest

from lamdis import Job, Lamdis, LamdisError, Posted

AWAITING = {
    "job": "observe-1",
    "kind": "observe",
    "status": "awaiting_payment",
    "pay_at": "https://checkout.example/cs_123",
    "token": "lbt_abc",
    "amount_minor": 800,
    "currency": "USD",
    "expires_at": "2026-09-03T00:00:00Z",
    "status_url": "https://exchange.lamdis.ai/v1/jobs/observe-1",
    "watch": "https://exchange.lamdis.ai/my/observe-1?t=lbt_abc",
    "note": "Send the person the pay link.",
}


class FakeExchange:
    """Records every request and answers from a route table."""

    def __init__(self):
        self.routes = {}
        self.calls = []
        fake = self

        class Handler(BaseHTTPRequestHandler):
            def _serve(self):
                length = int(self.headers.get("Content-Length") or 0)
                raw = self.rfile.read(length) if length else b""
                body = json.loads(raw) if raw else None
                fake.calls.append(
                    {
                        "method": self.command,
                        "path": self.path,
                        "headers": {k: v for k, v in self.headers.items()},
                        "body": body,
                    }
                )
                status, payload = fake.routes.get(
                    f"{self.command} {self.path}", (404, {"error": f"no route {self.command} {self.path}"})
                )
                out = json.dumps(payload).encode()
                self.send_response(status)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(out)))
                self.end_headers()
                self.wfile.write(out)

            do_GET = do_POST = _serve

            def log_message(self, *a):  # quiet
                pass

        self.server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()

    @property
    def url(self):
        host, port = self.server.server_address
        return f"http://{host}:{port}"

    def close(self):
        self.server.shutdown()
        self.server.server_close()


@pytest.fixture
def exchange():
    fx = FakeExchange()
    yield fx
    fx.close()


def test_observe_anonymous_returns_pay_link_and_token(exchange):
    exchange.routes["POST /v1/tasks"] = (200, AWAITING)
    x = Lamdis(base_url=exchange.url)
    posted = x.observe(
        "The 'For Lease' sign is still up",
        800,
        where="1200 Valencia St",
        lat=37.75,
        lon=-122.42,
        radius_m=150,
    )
    assert isinstance(posted, Posted)
    call = exchange.calls[0]
    assert call["method"] == "POST" and call["path"] == "/v1/tasks"
    assert "X-Lamdis-Key" not in call["headers"]
    assert "Authorization" not in call["headers"]
    assert call["headers"]["X-Lamdis-Posted-By"] == "agent"
    assert call["body"]["kind"] == "observe"
    assert call["body"]["fee_minor"] == 800
    assert call["body"]["radius_m"] == 150
    assert posted.job == "observe-1"
    assert posted.status == "awaiting_payment"
    assert posted.pay_at == AWAITING["pay_at"]
    assert posted.token == "lbt_abc"
    assert posted.watch == AWAITING["watch"]
    assert posted.amount_minor == 800
    assert posted.pay_usdc is None
    assert posted.raw == AWAITING


def test_pay_usdc_is_surfaced_when_the_rail_is_on(exchange):
    pay_usdc = {"address": "0xabc", "amount_usdc": "8.000123", "chain": "base", "chain_id": 8453, "contract": "0xusdc"}
    exchange.routes["POST /v1/tasks"] = (200, dict(AWAITING, pay_usdc=pay_usdc))
    posted = Lamdis(base_url=exchange.url).observe("p", 800)
    assert posted.pay_usdc == pay_usdc


def test_do_posts_kind_do_with_instructions(exchange):
    exchange.routes["POST /v1/tasks"] = (200, dict(AWAITING, job="do-1", kind="do"))
    posted = Lamdis(base_url=exchange.url).do(
        "The bins are behind the gate", "Wheel both bins through the side gate.", 1200, attempt_minor=300
    )
    body = exchange.calls[0]["body"]
    assert body["kind"] == "do"
    assert body["instructions"] == "Wheel both bins through the side gate."
    assert body["attempt_minor"] == 300
    assert posted.kind == "do"


def test_job_with_token_sends_bearer_everywhere(exchange):
    exchange.routes.update(
        {
            "GET /v1/jobs/observe-1": (200, {"job": "observe-1", "status": "awaiting_payment"}),
            "GET /v1/jobs/observe-1/receipt": (200, {"job": "observe-1", "accepted": True, "signature": "aa"}),
            "GET /v1/jobs/observe-1/evidence": (200, {"job": "observe-1", "files": []}),
            "POST /v1/jobs/observe-1/cancel": (200, {"job": "observe-1", "cancelled": True, "released_minor": 800}),
            "POST /v1/jobs/observe-1/release": (200, {"job": "observe-1", "released": 1, "status": "ok"}),
            "POST /v1/jobs/observe-1/hold": (200, {"job": "observe-1", "held": 1}),
            "GET /v1/jobs/observe-1/receipt/anchor?sha256=" + "ab" * 32: (200, {"job": "observe-1", "status": "pending"}),
        }
    )
    job = Lamdis(base_url=exchange.url).job("observe-1", "lbt_abc")
    assert isinstance(job, Job)
    assert job.status()["status"] == "awaiting_payment"
    assert job.receipt()["accepted"] is True
    assert job.evidence()["files"] == []
    assert job.cancel("plan changed")["cancelled"] is True
    assert job.release()["released"] == 1
    assert job.hold("not_done", "the gate is still open")["held"] == 1
    assert job.anchor("ab" * 32)["status"] == "pending"
    assert len(exchange.calls) == 7
    for c in exchange.calls:
        assert c["headers"]["Authorization"] == "Bearer lbt_abc"
        assert "X-Lamdis-Key" not in c["headers"]
    assert exchange.calls[3]["body"] == {"reason": "plan changed"}
    assert exchange.calls[5]["body"] == {"ground": "not_done", "reason": "the gate is still open"}


def test_hold_refuses_an_unknown_ground_locally(exchange):
    with pytest.raises(ValueError):
        Lamdis(base_url=exchange.url).job("j", "lbt_x").hold("meh", "not good enough")
    assert exchange.calls == []


def test_agent_key_and_funded_post(exchange):
    exchange.routes["POST /v1/tasks"] = (
        200,
        {"job": "do-2", "kind": "do", "board": "https://x/board", "escrowed": 1500,
         "expires": "2026-09-03T00:00:00Z", "status": "https://x/v1/jobs/do-2"},
    )
    posted = Lamdis(base_url=exchange.url, key="lam_sk_test").do("p", "i", 1500)
    assert exchange.calls[0]["headers"]["X-Lamdis-Key"] == "lam_sk_test"
    assert posted.escrowed_minor == 1500
    assert posted.expires_at == "2026-09-03T00:00:00Z"
    assert posted.token is None and posted.pay_at is None


def test_check_feasible_and_quote_post_v1_quote(exchange):
    quote = {"reachable": "several", "feasible": True}
    exchange.routes["POST /v1/quote"] = (200, quote)
    x = Lamdis(base_url=exchange.url)
    assert x.check_feasible(kind="do", skills=["ladder"], lat=1, lon=2) == quote
    assert x.quote(predicate="gutters clear") == quote
    assert [c["path"] for c in exchange.calls] == ["/v1/quote", "/v1/quote"]
    assert exchange.calls[0]["body"] == {"kind": "do", "skills": ["ladder"], "lat": 1, "lon": 2}


def test_board_gets_terms(exchange):
    exchange.routes["GET /v1/board"] = (
        200,
        {"work": [], "reviews_waiting": 0, "personalized": False,
         "terms": {"fee_bp": 0, "payout_threshold_minor": 2000, "currency": "USD"}},
    )
    board = Lamdis(base_url=exchange.url).board()
    assert board["terms"]["fee_bp"] == 0
    assert board["terms"]["payout_threshold_minor"] == 2000
    assert exchange.calls[0]["method"] == "GET"
    assert exchange.calls[0]["body"] is None


def test_job_without_token_relies_on_key(exchange):
    exchange.routes["GET /v1/jobs/do-2/bids"] = (200, {"job": "do-2", "bids": []})
    Lamdis(base_url=exchange.url, key="lam_sk_test").job("do-2").bids()
    assert "Authorization" not in exchange.calls[0]["headers"]
    assert exchange.calls[0]["headers"]["X-Lamdis-Key"] == "lam_sk_test"


def test_error_carries_status_and_sentence(exchange):
    exchange.routes["POST /v1/tasks"] = (400, {"error": "a job must pay for the work it asks for"})
    with pytest.raises(LamdisError) as ei:
        Lamdis(base_url=exchange.url).observe("x", 0)
    assert ei.value.status == 400
    assert str(ei.value) == "a job must pay for the work it asks for"
    assert ei.value.body == {"error": "a job must pay for the work it asks for"}


def test_trailing_slash_is_stripped(exchange):
    exchange.routes["GET /v1/board"] = (200, {"work": []})
    Lamdis(base_url=exchange.url + "/").board()
    assert exchange.calls[0]["path"] == "/v1/board"
