/// <reference types="cypress" />
/// <reference types="cypress-file-upload" />
/// <reference types="cypress-wait-until" />

describe('Invoice processing end‑to‑end flow', () => {
  const user = {
    email: 'accountant@example.com',
    password: 'Password123!'
  };

  const invoiceFixture = 'sample_invoice.pdf';
  const expectedVendor = 'Acme Corp';
  const expectedTotal = '1234.56';

  /**
   * Reset the test database and seed a known user before the suite runs.
   * The backend exposes a thin test‑only API for this purpose.
   */
  before(() => {
    cy.request('POST', '/api/test/reset');
    cy.request('POST', '/api/test/seed', { user });
  });

  /**
   * Preserve authentication cookies/JWT between tests in the same spec.
   */
  beforeEach(() => {
    Cypress.Cookies.preserveOnce('session_id', 'jwt');
  });

  it('logs in, uploads an invoice, waits for processing, validates UI data and webhook payload', () => {
    // -----------------------------------------------------------------------
    // 1️⃣ Login
    // -----------------------------------------------------------------------
    cy.intercept('POST', '/api/auth/login').as('loginRequest');

    cy.visit('/login');

    cy.get('[data-testid="email-input"]').type(user.email);
    cy.get('[data-testid="password-input"]').type(user.password);
    cy.get('[data-testid="login-button"]').click();

    cy.wait('@loginRequest')
      .its('response.statusCode')
      .should('eq', 200);

    cy.url().should('include', '/dashboard');

    // -----------------------------------------------------------------------
    // 2️⃣ Prepare intercepts for the rest of the flow
    // -----------------------------------------------------------------------
    // Capture the webhook dispatch that the backend will fire after extraction.
    cy.intercept('POST', '/api/webhooks/*/dispatch', (req) => {
      // Alias for later waiting/assertion.
      req.alias = 'webhookDispatch';
    }).as('webhookDispatch');

    // Capture the invoice upload request.
    cy.intercept('POST', '/api/invoices').as('uploadInvoice');

    // -----------------------------------------------------------------------
    // 3️⃣ Navigate to the Upload page
    // -----------------------------------------------------------------------
    cy.get('[data-testid="nav-upload"]').click();
    cy.url().should('include', '/upload');

    // -----------------------------------------------------------------------
    // 4️⃣ Upload a PDF via the Dropzone component
    // -----------------------------------------------------------------------
    cy.get('[data-testid="upload-dropzone"] input[type="file"]')
      .attachFile(invoiceFixture, { subjectType: 'input' });

    // Verify preview appears.
    cy.get('[data-testid="file-preview"]')
      .should('contain', invoiceFixture);

    // Progress bar should be visible while the request is in flight.
    cy.get('[data-testid="upload-progress"]').should('exist');

    // Wait for the backend to acknowledge the upload.
    cy.wait('@uploadInvoice')
      .its('response.statusCode')
      .should('eq', 201);

    // Progress bar disappears after the request completes.
    cy.get('[data-testid="upload-progress"]').should('not.exist');

    // -----------------------------------------------------------------------
    // 5️⃣ Wait for asynchronous processing (OCR → LLM → webhook)
    // -----------------------------------------------------------------------
    // The invoice appears in the table with a temporary “Processing” badge.
    cy.get('[data-testid="invoice-table"]')
      .contains('td', invoiceFixture)
      .parents('tr')
      .as('invoiceRow');

    // Ensure the status chip initially shows “Processing”.
    cy.get('@invoiceRow')
      .find('[data-testid="status-chip"]')
      .should('contain', 'Processing');

    // Poll until the status becomes “Processed”. 30 s timeout is generous for CI.
    cy.waitUntil(() =>
      cy.get('@invoiceRow')
        .find('[data-testid="status-chip"]')
        .invoke('text')
        .then((txt) => txt.trim() === 'Processed')
    , {
      timeout: 30000,
      interval: 2000,
      errorMsg: 'Invoice never reached Processed state',
    });

    // -----------------------------------------------------------------------
    // 6️⃣ Verify extracted data is displayed in the UI
    // -----------------------------------------------------------------------
    cy.get('@invoiceRow')
      .within(() => {
        cy.get('[data-testid="vendor-cell"]')
          .should('contain', expectedVendor);
        cy.get('[data-testid="total-cell"]')
          .should('contain', expectedTotal);
      });

    // -----------------------------------------------------------------------
    // 7️⃣ Assert the outbound webhook payload
    // -----------------------------------------------------------------------
    cy.wait('@webhookDispatch')
      .its('request')
      .then((req) => {
        // Payload shape validation
        expect(req.body).to.have.property('invoiceId');
        expect(req.body.vendor).to.equal(expectedVendor);
        expect(req.body.total).to.equal(expectedTotal);

        // HMAC header presence – actual constant‑time verification is done server‑side.
        expect(req.headers).to.have.property('x-hmac-signature');
        // Optional: basic format check (hex string of 64 chars for SHA‑256)
        expect(req.headers['x-hmac-signature']).to.match(/^[a-f0-9]{64}$/i);
      });
  });
});