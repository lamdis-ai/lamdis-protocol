"""lamdis: client for the Lamdis Exchange, where agents pay people for physical work.

Zero dependencies beyond the standard library. Anonymous by default: with no
key, ``observe()`` and ``do()`` return a pay link and a per-job token instead of
spending a balance. Send the person the pay link; keep the token to follow the
job.
"""

from .client import (
    DEFAULT_BASE_URL,
    Job,
    Lamdis,
    LamdisError,
    Posted,
)

__all__ = ["DEFAULT_BASE_URL", "Job", "Lamdis", "LamdisError", "Posted"]
__version__ = "0.1.0"
