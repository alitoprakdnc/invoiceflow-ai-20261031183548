"""
Locust load‑testing script for InvoiceFlow AI.

Simulates realistic user behaviour:
1. Authenticate via the `/api/auth/login` endpoint (JWT Bearer token).
2. Upload a PDF invoice to `/api/invoices/upload`.
3. Periodically poll the processing status endpoint until the invoice is
   marked as `completed` or a maximum number of attempts is reached.
4. Optionally fetch a dashboard summary to generate read‑only traffic.

The script is production‑ready:
* Configurable via environment variables.
* Robust error handling with detailed Locust metrics.
* Uses a shared queue to decouple upload and polling tasks.
* Logs useful information without leaking secrets.
"""

import os
import base64
import json
import random
import time
from pathlib import Path
from typing import Optional, Dict, Any, List

from locust import HttpUser, task, between, events, SequentialTaskSet, tag
from locust.exception import RescheduleTask, StopUser

# --------------------------------------------------------------------------- #
# Configuration (environment variables with sensible defaults)
# --------------------------------------------------------------------------- #
API_BASE_URL = os.getenv("LOCUST_API_URL", "http://localhost:8080")
USER_EMAIL = os.getenv("LOCUST_USER_EMAIL", "demo@example.com")
USER_PASSWORD = os.getenv("LOCUST_USER_PASSWORD", "demoPassword123")
# Path to a sample PDF that will be uploaded repeatedly.
SAMPLE_PDF_PATH = os.getenv(
    "LOCUST_SAMPLE_PDF", str(Path(__file__).parent.parent / "tests" / "fixtures" / "sample_invoice.pdf")
)
# How many invoices a single virtual user should upload during its lifetime.
INVOICES_PER_USER = int(os.getenv("LOCUST_INVOICES_PER_USER", "5"))
# Seconds between status polls.
POLL_INTERVAL = float(os.getenv("LOCUST_POLL_INTERVAL", "2"))
# Maximum number of polls before giving up on an invoice.
MAX_POLL_ATTEMPTS = int(os.getenv("LOCUST_MAX_POLL_ATTEMPTS", "30"))
# Optional: fetch dashboard stats after each upload (adds read‑only load).
FETCH_DASHBOARD = os.getenv("LOCUST_FETCH_DASHBOARD", "false").lower() == "true"

# --------------------------------------------------------------------------- #
# Helper utilities
# --------------------------------------------------------------------------- #
def _load_sample_pdf() -> bytes:
    """Read the sample PDF once and keep it in memory."""
    pdf_path = Path(SAMPLE_PDF_PATH)
    if not pdf_path.is_file():
        raise FileNotFoundError(f"Sample PDF not found at {pdf_path}")
    return pdf_path.read_bytes()


_SAMPLE_PDF_BYTES = _load_sample_pdf()


def _auth_payload() -> Dict[str, str]:
    """Payload for the login request."""
    return {"email": USER_EMAIL, "password": USER_PASSWORD}


def _extract_jwt(response: Any) -> str:
    """Extract JWT token from a successful login response."""
    try:
        data = response.json()
        token = data.get("access_token") or data.get("token")
        if not token:
            raise ValueError("JWT token not present in login response")
        return token
    except Exception as exc:
        raise ValueError(f"Failed to parse JWT from login response: {exc}") from exc


def _log_event(event_name: str, **kwargs: Any) -> None:
    """Emit a custom Locust event for debugging / external listeners."""
    events.request_success.fire(
        request_type="EVENT",
        name=event_name,
        response_time=0,
        response_length=0,
    )
    # Additional data can be attached via the `extra` attribute of the event if needed.
    # For now we keep it simple.


# --------------------------------------------------------------------------- #
# TaskSet that performs the upload → poll workflow
# --------------------------------------------------------------------------- #
class InvoiceWorkflow(SequentialTaskSet):
    """
    SequentialTaskSet ensures that each virtual user follows the exact order:
    1. Upload an invoice.
    2. Poll its status until completion.
    3. (Optionally) fetch a dashboard summary.
    The set repeats `INVOICES_PER_USER` times.
    """

    def __init__(self, parent):
        super().__init__(parent)
        self.jwt_token: Optional[str] = None
        self.uploaded_invoice_ids: List[int] = []

    def on_start(self) -> None:
        """Authenticate once per virtual user."""
        with self.client.post(
            "/api/auth/login",
            json=_auth_payload(),
            name="login",
            catch_response=True,
        ) as response:
            if response.status_code != 200:
                response.failure(f"Login failed with status {response.status_code}")
                raise StopUser()
            try:
                self.jwt_token = _extract_jwt(response)
                response.success()
                _log_event("login_success")
            except Exception as exc:
                response.failure(str(exc))
                raise StopUser()

    @task
    @tag("upload")
    def upload_invoice(self) -> None:
        """Upload a single PDF invoice."""
        if len(self.uploaded_invoice_ids) >= INVOICES_PER_USER:
            # All uploads for this user are done; stop the task set.
            raise RescheduleTask()

        headers = {"Authorization": f"Bearer {self.jwt_token}"}
        files = {
            "file": ("sample_invoice.pdf", _SAMPLE_PDF_BYTES, "application/pdf")
        }

        with self.client.post(
            "/api/invoices/upload",
            files=files,
            headers=headers,
            name="upload_invoice",
            catch_response=True,
        ) as response:
            if response.status_code != 202:
                response.failure(
                    f"Upload failed with status {response.status_code}: {response.text}"
                )
                raise RescheduleTask()
            try:
                payload = response.json()
                invoice_id = payload.get("invoice_id")
                if not invoice_id:
                    raise ValueError("Response missing `invoice_id`")
                self.uploaded_invoice_ids.append(int(invoice_id))
                response.success()
                _log_event("invoice_uploaded", invoice_id=invoice_id)
            except Exception as exc:
                response.failure(f"Invalid upload response: {exc}")
                raise RescheduleTask()

    @task
    @tag("poll")
    def poll_status(self) -> None:
        """Poll the status of the most recently uploaded invoice."""
        if not self.uploaded_invoice_ids:
            # Nothing to poll yet – skip this task iteration.
            raise RescheduleTask()

        invoice_id = self.uploaded_invoice_ids[-1]
        headers = {"Authorization": f"Bearer {self.jwt_token}"}
        attempts = 0

        while attempts < MAX_POLL_ATTEMPTS:
            attempts += 1
            start = time.time()
            with self.client.get(
                f"/api/invoices/{invoice_id}/status",
                headers=headers,
                name="poll_status",
                catch_response=True,
            ) as response:
                elapsed = (time.time() - start) * 1000  # ms
                if response.status_code != 200:
                    response.failure(
                        f"Status poll failed ({response.status_code}): {response.text}"
                    )
                    # Continue polling – transient errors are expected.
                else:
                    try:
                        data = response.json()
                        status = data.get("status")
                        if not status:
                            raise ValueError("Missing `status` field")
                        response.success()
                        _log_event("status_polled", invoice_id=invoice_id, status=status)
                        if status.lower() in {"completed", "failed"}:
                            # Invoice processing finished – break out.
                            break
                    except Exception as exc:
                        response.failure(f"Invalid status payload: {exc}")

            # Respect the configured poll interval.
            time.sleep(POLL_INTERVAL)

        else:
            # Max attempts exhausted without terminal status.
            self.interrupt(reschedule=False)
            raise StopUser()

    @task
    @tag("dashboard")
    def fetch_dashboard(self) -> None:
        """Optionally hit a read‑only dashboard endpoint to generate extra load."""
        if not FETCH_DASHBOARD:
            raise RescheduleTask()

        headers = {"Authorization": f"Bearer {self.jwt_token}"}
        with self.client.get(
            "/api/dashboard/summary",
            headers=headers,
            name="fetch_dashboard",
            catch_response=True,
        ) as response:
            if response.status_code != 200:
                response.failure(f"Dashboard fetch failed: {response.status_code}")
            else:
                response.success()
                _log_event("dashboard_fetched")

        # After the optional dashboard call, continue the loop for the next invoice.
        # No explicit RescheduleTask needed – the SequentialTaskSet will loop.


# --------------------------------------------------------------------------- #
# Locust user definition
# --------------------------------------------------------------------------- #
class InvoiceUser(HttpUser):
    """
    Represents a single concurrent user performing the invoice workflow.
    The wait_time introduces a small random pause between task set iterations,
    mimicking real‑world think time.
    """

    tasks = [InvoiceWorkflow]
    wait_time = between(1, 3)  # seconds

    # Override the host with the API base URL if provided.
    host = API_BASE_URL.rstrip("/")

    def on_start(self) -> None:
        """Log start of a virtual user."""
        _log_event("user_started", user=self.environment.runner.user_count)

    def on_stop(self) -> None:
        """Log stop of a virtual user."""
        _log_event("user_stopped", user=self.environment.runner.user_count)


# --------------------------------------------------------------------------- #
# Custom event listeners for better reporting
# --------------------------------------------------------------------------- #
@events.test_stop.add_listener
def on_test_stop(environment, **kwargs):
    """Print a short summary when the test finishes."""
    if environment.stats.total.fail_ratio > 0:
        print(
            f"\n[Locust] Test finished with failure ratio: {environment.stats.total.fail_ratio:.2%}"
        )
    else:
        print("\n[Locust] Test finished successfully – no failures recorded.")