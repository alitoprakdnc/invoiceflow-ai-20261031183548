# InvoiceFlow AI

A SaaS platform that automates the ingestion, OCR, AI‑enhanced extraction and categorisation of invoices, receipts and other financial documents. Users upload documents via a modern web UI or API; the system extracts line‑items, taxes, totals and vendor data using Tesseract OCR + a local LLM (Ollama) for fuzzy parsing, validates the data, stores it in a relational store and pushes structured results to downstream accounting systems through secure, HMAC‑signed webhooks. The product includes a full OpenAPI‑documented API, health‑check endpoints, role‑based access, audit logs and a built‑in monitor for GitHub Actions queue starvation for teams that run CI/CD pipelines on the same cloud project.

## Technologies

- Go (Gin)
- Python (FastAPI for LLM wrapper)
- React + TypeScript
- Tailwind CSS
- PostgreSQL
- Redis
- Google Cloud Tasks (push queue)
- Google Cloud Secret Manager
- Tesseract OCR
- Ollama (local LLM) / OpenAI compatible API
- Docker & Docker‑Compose
- Kubernetes (optional production)
- OpenAPI 3.0

## Features

- Secure multi‑user authentication (JWT) with role‑based permissions (admin, accountant, viewer).
- Drag‑and‑drop document upload with client‑side preview and progress bar.
- Automatic OCR via Tesseract, followed by LLM‑enhanced entity extraction (vendor, date, line items, taxes).
- Configurable validation rules (e.g., total = sum(line items) ± tolerance).
- Async processing pipeline powered by Google Cloud Tasks; status updates via WebSocket.
- Audit log of every upload, extraction, validation and webhook dispatch.
- Outbound webhook engine with constant‑time HMAC verification for receiving systems.
- OpenAPI 3.0 spec with interactive Swagger UI (embedded via embed.FS).
- Health‑check endpoint (/api/health) exposing liveness, readiness and queue depth.
- Dashboard with charts (documents processed per day, error rates, webhook success).
- GitHub Actions queue monitor that surfaces starvation metrics and alerts.
- Admin UI for secret management integration with Google Secret Manager (API keys, LLM tokens).
- Docker‑Compose for one‑click local development and CI pipelines.

## Architecture

The system follows a modular micro‑service style while being deployable as a single Docker‑Compose stack for SaaS.

- **API Gateway (Go/Gin)** – Handles authentication, request routing, health checks, OpenAPI serving, and forwards heavy‑weight jobs to the task queue.
- **Task Queue (Google Cloud Tasks)** – Push‑based queue that triggers background workers for OCR and LLM extraction.
- **OCR Service (Go)** – Wraps Tesseract, stores raw text in PostgreSQL.
- **LLM Service (Python/FastAPI)** – Thin wrapper around Ollama (or OpenAI) that receives raw text, returns structured JSON, and signs responses.
- **Webhook Dispatcher (Go)** – Sends POST requests to user‑configured endpoints, adds HMAC header, retries with exponential back‑off, logs outcomes.
- **Data Layer (PostgreSQL + Redis)** – PostgreSQL stores users, documents, extracted entities, audit logs; Redis caches session tokens and queue metrics.
- **Auth & RBAC (Go middleware)** – JWT validation, role checks, token revocation via Redis.
- **Secret Manager Adapter (Go)** – Pulls API keys / LLM tokens from Google Secret Manager at startup.
- **Monitoring Service (Go)** – Exposes Prometheus metrics, includes a GitHub Actions queue starvation detector that polls the GitHub API.
- **Frontend (React/TS)** – SPA built with Vite, Tailwind, React‑Query for data fetching, Chart.js for dashboards, and WebSocket listener for job status.
- **CI/CD** – GitHub Actions run unit, integration and Cypress e2e tests; the queue monitor helps teams spot bottlenecks.

All components are containerised; the production deployment can be split into separate pods/services on Kubernetes, but the dev environment runs with a single `docker-compose.yml`.

## Product Design

A clean, premium SaaS look built on Tailwind CSS 3 with a custom design token set:
- **Palette**: Primary #0A74DA, Secondary #14B8A6, Accent #F59E0B, Neutral greys from #F9FAFB to #111827.
- **Typography**: Inter (400/500/600) for UI, Roboto Mono for code snippets.
- **Spacing**: 4‑px base unit, consistent use of `space-x-4`, `p-6` etc.
- **Components**: Card, Table, Modal, Dropzone, Badge, Toast, Loading Skeleton, Switch, Pagination.
- **Interaction**: Subtle elevation (shadow‑md) on hover, focus rings, disabled opacity.
- **Responsive**: Mobile‑first breakpoints (sm ≥640px, md ≥768px, lg ≥1024px). The dashboard collapses side‑nav into a hamburger on small screens; tables become scrollable.
- **Accessibility**: ARIA labels, keyboard navigation, color contrast ≥4.5:1.
- **Theming**: Light and dark mode toggled via CSS variables; persisted in localStorage.
The UI is built as reusable React components under `src/components/` and follows the atomic design principle, enabling rapid feature addition without visual regression.

## Testing

Comprehensive testing strategy:
- **Unit Tests**: Go `*_test.go` using `testing` + `testify`; Python tests with `pytest`; React component tests with Jest + React Testing Library.
- **Integration Tests**: Go tests that spin up an in‑memory PostgreSQL (via `testcontainers`) and exercise API endpoints, including webhook signing.
- **End‑to‑End Tests**: Cypress suite covering upload flow, OCR/LLM processing, dashboard filters, and webhook configuration.
- **Static Analysis**: `golangci-lint`, `flake8`, `eslint` + `prettier`.
- **CI Pipeline**: GitHub Actions run lint → unit → integration → Cypress on each PR; on merge, Docker images are built and pushed to a registry.
- **Performance Tests**: Locust script (in `scripts/locustfile.py`) to simulate 100 concurrent uploads, ensuring queue depth stays under threshold.
- **Security Tests**: OWASP ZAP scan against the dev deployment, plus unit tests for HMAC constant‑time verification.
All test artifacts are stored under `tests/` and `scripts/`.

## Status

Generated by QuarkForge. Review and test before deployment. Automated test status is recorded in the generation logs.
