import React from "react";
import { render, screen, within, fireEvent } from "@testing-library/react";
import "@testing-library/jest-dom";
import InvoiceTable from "@/components/InvoiceTable";

/**
 * Mock invoice data used across tests.
 * The shape mirrors the type expected by `InvoiceTable`.
 */
const mockInvoices = [
  {
    id: "inv-001",
    vendor: "Acme Corp",
    date: "2024-09-15",
    total: 1250.5,
    status: "Pending",
  },
  {
    id: "inv-002",
    vendor: "Beta LLC",
    date: "2024-08-01",
    total: 300.0,
    status: "Processed",
  },
  {
    id: "inv-003",
    vendor: "Gamma Ltd",
    date: "2024-10-05",
    total: 780.75,
    status: "Error",
  },
];

/**
 * Helper to extract the date string from a rendered table row.
 */
function getRowDate(row: HTMLElement): string {
  // The component renders the date cell with a `data-testid="invoice-date"` attribute.
  return within(row).getByTestId("invoice-date").textContent?.trim() ?? "";
}

/**
 * Helper to extract the status chip element from a rendered table row.
 */
function getRowStatusChip(row: HTMLElement): HTMLElement {
  // The status chip is rendered with `data-testid="invoice-status"` for easy querying.
  return within(row).getByTestId("invoice-status");
}

/**
 * Expected Tailwind background classes for each status.
 * These are defined in the design system (green for processed, yellow for pending, red for error).
 */
const statusColorClasses: Record<string, string> = {
  Processed: "bg-green-100",
  Pending: "bg-yellow-100",
  Error: "bg-red-100",
};

describe("InvoiceTable component", () => {
  beforeEach(() => {
    render(<InvoiceTable invoices={mockInvoices} />);
  });

  test("renders the correct number of rows", () => {
    // All rows (including the header) have role="row".
    const rows = screen.getAllByRole("row");
    // Header row + one row per invoice.
    expect(rows).toHaveLength(mockInvoices.length + 1);
  });

  test("displays invoice data correctly", () => {
    mockInvoices.forEach((inv) => {
      // Vendor name appears somewhere in the table.
      expect(screen.getByText(inv.vendor)).toBeInTheDocument();
      // Total amount is formatted with two decimals and a leading $.
      const formattedTotal = `$${inv.total.toFixed(2)}`;
      expect(screen.getByText(formattedTotal)).toBeInTheDocument();
    });
  });

  test("status chips have correct colors", () => {
    const rows = screen.getAllByRole("row").slice(1); // skip header
    rows.forEach((row) => {
      const chip = getRowStatusChip(row);
      const statusText = chip.textContent?.trim() ?? "";
      const expectedClass = statusColorClasses[statusText];
      expect(expectedClass).toBeDefined();
      expect(chip).toHaveClass(expectedClass);
    });
  });

  test("sorting by date works (ascending → descending)", () => {
    // Find the Date column header – the component uses a button with the column name.
    const dateHeader = screen.getByRole("button", { name: /date/i });
    // Initial click → ascending order.
    fireEvent.click(dateHeader);

    let rows = screen.getAllByRole("row").slice(1);
    let datesAsc = rows.map(getRowDate);
    // Expect dates to be sorted from earliest to latest.
    const sortedAsc = [...mockInvoices]
      .map((i) => i.date)
      .sort((a, b) => a.localeCompare(b));
    expect(datesAsc).toEqual(sortedAsc);

    // Second click → descending order.
    fireEvent.click(dateHeader);
    rows = screen.getAllByRole("row").slice(1);
    const datesDesc = rows.map(getRowDate);
    const sortedDesc = [...sortedAsc].reverse();
    expect(datesDesc).toEqual(sortedDesc);
  });
});