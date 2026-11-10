# Lamdis cases API (insurance denial appeals)

Lamdis helps one person fight one health-insurance denial at a time. A **case**
holds the denial letter and everything else about it, the deadlines worked out
from the rules that apply, an honest read of the odds, a checklist, an appeal
letter in which every factual sentence is checked against the documents, and a
link the doctor's office can use to add their letter.

All routes below sit under `https://app.lamdis.ai` and, except where marked
public, take `Authorization: Bearer <token>` where the token comes from
`POST /app/api/start` (a guest; no sign-in needed). Bodies and responses are
JSON unless noted. Errors are `{"error": "words a person can read"}` with a
4xx/5xx status.

## Start and files (existing)

- `POST /app/api/start` → `{ "token": "...", ... }` Public. Creates a guest.
- `POST /app/api/upload?name=<filename>` raw bytes body (PDF, PNG, JPEG, WebP, GIF, HEIC is NOT accepted: convert to JPEG on the phone; 20 MB max) → `FileRef {id, name, type, size, url}`
- `GET /app/api/file/{id}` → the bytes.

## Cases

- `POST /app/api/cases` `{ "files": ["<fileId>", ...] }` → `Case` with `status: "reading"`. The files are the denial letter (all pages; photos of pages are fine). Reading runs in the background; poll `GET /app/api/cases/{id}` every 2 s until `status` is not `reading`.
- `GET /app/api/cases` → `{ "cases": [CaseSummary] }`, newest first.
- `GET /app/api/cases/{id}` → `Case`.
- `POST /app/api/cases/{id}/docs` `{ "files": [...], "kind": "denial|policy|doctor_letter|records|insurer_reply|other" }` → `Case` (status goes back to `reading` while the new pages are read).
- `POST /app/api/cases/{id}/answers` `{ "answers": { "<questionId>": "<value>" } }` → `Case` (deadlines and odds recomputed). Answer values: for `choice` questions one of the option `value`s; for `date` questions `YYYY-MM-DD`; for `text` questions free text.
- `POST /app/api/cases/{id}/letter` `{}` → `Case` with `letter.status: "drafting"`; poll until `drafted` or `error`.
- `POST /app/api/cases/{id}/letter/sentence` `{ "sentence": "<id>", "keep": true|false }` → `Case`. Confirms a flagged sentence as true (`keep: true`) or drops it (`keep: false`).
- `GET /app/api/cases/{id}/letter.html` → printable HTML of the current letter (text/html). Phones render it to PDF; browsers print it.
- `POST /app/api/cases/{id}/invite` `{}` → `{ "url": "https://app.lamdis.ai/d/<acct>/<cap>" }` the doctor's-office link. Idempotent: returns the existing link if there is one.
- `POST /app/api/cases/{id}/event` `{ "kind": "sent|decision|note", "date": "YYYY-MM-DD", "outcome": "won|partial|lost", "note": "" }` → `Case`. `sent` records that the appeal went out (starts the insurer's decision clock); `decision` records the result (`outcome` required).
- `POST /app/api/cases/{id}/step` `{ "step": "<id>", "done": true|false }` → `Case`.
- `POST /app/api/cases/{id}/delete` `{}` → `{ "ok": true }`. Deletes the case and its files for good.
- `POST /app/api/cases/{id}/retry` `{}` → `Case`. Re-runs a failed read (`status: "error"`) or a failed letter (`letter.status: "error"`).
- Event kind `reopen` clears a recorded decision.

## Reminders and sign-in (existing)

- `GET /app/api/email` → `{ "email", "confirmed", "can_send" }`.
- `POST /app/api/signin/email` `{ "email" }` → `{ ok, sent_to, existing }`. If `existing` is false, confirm the address onto this account with `POST /app/api/email/confirm` `{ "code" }`. If true, the address belongs to another account: `POST /app/api/signin/email/confirm` `{ "email", "code" }` → `{ "token" }`, which replaces the stored token.
- A confirmed email gets reminders 14, 7 and 2 days before, and on the day of, each deadline (8 a.m.–8 p.m. US Eastern), and a note when the doctor's office uploads.

## Doctor's office link (public, capability URL)

- `GET /d/{acct}/{cap}` → HTML page: what was denied, the deadline, a draft letter of medical necessity to edit and sign, and an upload box.
- `POST /d/{acct}/{cap}/upload?name=<filename>` raw bytes → `{ "ok": true }`. Adds the file to the case as `doctor_letter`.
- `GET /d/{acct}/{cap}/lmn.html` → printable draft letter of medical necessity.

## Shapes

```jsonc
// CaseSummary
{ "id": "c_…", "title": "Zepbound · UnitedHealthcare", "status": "ready",
  "next_deadline": Deadline | null, "outcome": "" , "updated": "RFC3339" }

// Case
{
  "id": "c_…", "created": "RFC3339", "updated": "RFC3339",
  "title": "Zepbound · UnitedHealthcare",
  "status": "reading|needs_answers|ready|drafting|drafted|sent|decided|error",
  "error": "",                       // set when status is error
  "docs": [ { "id": "<fileId>", "name": "letter.pdf", "type": "application/pdf",
              "kind": "denial", "added_by": "you|doctor", "added": "RFC3339",
              "url": "/app/api/file/<fileId>", "read": true } ],
  "facts": {                         // each fact may be absent
    "insurer":        Fact, "plan_name": Fact, "member_id": Fact, "claim_number": Fact,
    "patient":        Fact, "service":   Fact, "service_date": Fact, "notice_date": Fact,
    "reason":         Fact, "stated_deadline": Fact, "appeal_address": Fact,
    "appeal_fax":     Fact, "appeal_phone": Fact, "provider": Fact
  },
  "plan":   { "type": "employer|government_employer|marketplace|medicare_advantage|medicare_part_d|original_medicare|medicaid_managed_care|medicaid|fehb|tricare|unknown",
              "label": "Plan through a private employer", "sure": true, "why": "…" },
  "denial": { "category": "medical_necessity|experimental|out_of_network|no_prior_auth|exclusion|step_therapy|coding|eligibility|other",
              "label": "Not medically necessary", "plain": "Plain-English explanation.", "level": "first|second|final" , "urgent": false },
  "questions": [ { "optional": false, "id": "plan_source", "text": "Where does this insurance come from?", "why": "…",
                   "kind": "choice|date|text", "options": [ { "value": "employer", "label": "A job (mine or a family member's)" } ] } ],
  "deadlines": [ Deadline ],
  "next_deadline": Deadline | null,
  "odds":  { "level": "strong|possible|long_shot|unclear", "headline": "…",
             "reasons": [ { "text": "…", "quote": "…", "doc": "<fileId>" } ],
             "alternatives": [ "…" ] },
  "steps": [ { "id": "get_file", "title": "…", "detail": "…", "done": false,
               "action": "invite_doctor|draft_letter|send|record_decision|external_review|none" } ],
  "letter": {
    "status": "none|drafting|drafted|error", "error": "", "generated": "RFC3339",
    "to": "Appeals address as one block", "subject": "…",
    "sentences": [ { "id": "s1", "para": 0, "text": "…", "kind": "fact|argument|request|courtesy",
                     "status": "verified|plain|unsupported|confirmed|removed",  // plain = a request or courtesy with nothing to check
                     "cites": [ { "doc": "<fileId>|rule:<ruleId>", "title": "Denial letter, page 1", "quote": "…", "url": "", "found": true } ] } ],
    "enclosures": [ "…" ],
    "checked": { "facts": 9, "verified": 8, "flagged": 1 }
  },
  "doctor": { "link": "", "invited": "RFC3339|''", "uploaded": false },
  "timeline": [ { "at": "RFC3339", "text": "…" } ],
  "outcome": { "result": "|won|partial|lost", "date": "", "note": "" }
}

// Fact
{ "value": "UnitedHealthcare", "quote": "verbatim text from the document", "doc": "<fileId>", "verified": true }

// Deadline
{ "id": "appeal", "label": "Ask your plan to reconsider", "date": "YYYY-MM-DD", "days_left": 59,
  "basis": "65 days from the date on your notice", "kind": "appeal|decision|external|other",
  "source": { "title": "42 CFR 422.582", "url": "https://www.ecfr.gov/…" }, "from_letter": false }
```

## Rules

- `verified: true` on a fact means the quote was found word for word in the
  text Lamdis read from that document. A sentence in the letter is `verified`
  only when every one of its quotes was found. `unsupported` sentences are left
  out of `letter.html` until the person confirms them.
- When the letter itself states a deadline and it is earlier than the rule's,
  the earlier date wins and both are shown.
- Nothing here is legal or medical advice; Lamdis drafts, the person decides
  and sends.
