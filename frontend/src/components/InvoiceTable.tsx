import React, { useMemo, MouseEvent } from "react";
import { Invoice } from "@/types";
import { format } from "date-fns";
import { Badge } from "@/components/Badge";
import { Skeleton } from "@/components/Skeleton";

type SortDirection = "asc" | "desc";

interface SortState {
  column: keyof Invoice;
  direction: SortDirection;
}

interface InvoiceTableProps {
  /** List of invoices to display */
  invoices: Invoice[];
  /** Current sorting state */
  sort: SortState;
  /** Callback when a column header is clicked to change sorting */
  onSortChange: (column: keyof Invoice) => void;
  /** Optional row click handler */
  onRowClick?: (invoice: Invoice) => void;
  /** Loading flag – shows skeleton rows while true */
  loading?: boolean;
  /** Error message – displayed above the table when present */
  error?: string;
}

/**
 * Returns a human‑readable label for a column key.
 */
const columnLabel = (col: keyof Invoice): string => {
  switch (col) {
    case "id":
      return "ID";
    case "vendor":
      return "Vendor";
    case "date":
      return "Date";
    case "total":
      return "Total";
    case "status":
      return "Status";
    default:
      return (col as string).charAt(0).toUpperCase() + (col as string).slice(1);
  }
};

/**
 * Returns a Tailwind background class for a given invoice status.
 */
const statusBadgeStyle = (status: Invoice["status"]): string => {
  switch (status) {
    case "pending":
      return "bg-gray-100 text-gray-800";
    case "processing":
      return "bg-blue-100 text-blue-800";
    case "completed":
      return "bg-green-100 text-green-800";
    case "failed":
      return "bg-red-100 text-red-800";
    default:
      return "bg-gray-100 text-gray-800";
  }
};

/**
 * Renders a sortable table header cell.
 */
const SortableHeader: React.FC<{
  column: keyof Invoice;
  label: string;
  sort: SortState;
  onClick: (col: keyof Invoice) => void;
}> = ({ column, label, sort, onClick }) => {
  const isActive = sort.column === column;
  const direction = isActive ? sort.direction : undefined;

  const handleClick = (e: MouseEvent) => {
    e.preventDefault();
    onClick(column);
  };

  const ariaSort = isActive ? (direction === "asc" ? "ascending" : "descending") : "none";

  return (
    <th
      scope="col"
      className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider cursor-pointer select-none"
      onClick={handleClick}
      aria-sort={ariaSort}
    >
      <div className="flex items-center space-x-1">
        <span>{label}</span>
        {isActive && (
          <svg
            className={`w-3 h-3 transition-transform ${
              direction === "asc" ? "rotate-180" : ""
            }`}
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
          </svg>
        )}
      </div>
    </th>
  );
};

export const InvoiceTable: React.FC<InvoiceTableProps> = ({
  invoices,
  sort,
  onSortChange,
  onRowClick,
  loading = false,
  error,
}) => {
  const columns: (keyof Invoice)[] = useMemo(
    () => ["id", "vendor", "date", "total", "status"],
    []
  );

  const handleHeaderClick = (col: keyof Invoice) => {
    onSortChange(col);
  };

  const handleRowClick = (invoice: Invoice) => {
    if (onRowClick) {
      onRowClick(invoice);
    }
  };

  const renderRows = () => {
    if (loading) {
      // Render 5 skeleton rows
      return Array.from({ length: 5 }).map((_, idx) => (
        <tr key={`skeleton-${idx}`} className="border-b border-gray-200">
          {columns.map((col) => (
            <td key={col} className="px-4 py-2">
              <Skeleton className="h-4 w-full" />
            </td>
          ))}
        </tr>
      ));
    }

    if (invoices.length === 0) {
      return (
        <tr>
          <td colSpan={columns.length} className="px-4 py-8 text-center text-gray-500">
            No invoices found.
          </td>
        </tr>
      );
    }

    return invoices.map((inv) => (
      <tr
        key={inv.id}
        className="border-b border-gray-200 hover:bg-gray-50 cursor-pointer"
        onClick={() => handleRowClick(inv)}
      >
        <td className="px-4 py-2 text-sm text-gray-900">{inv.id}</td>
        <td className="px-4 py-2 text-sm text-gray-700">{inv.vendor}</td>
        <td className="px-4 py-2 text-sm text-gray-700">
          {inv.date ? format(new Date(inv.date), "PPP") : "-"}
        </td>
        <td className="px-4 py-2 text-sm font-medium text-gray-900">
          {inv.total != null ? `$${inv.total.toFixed(2)}` : "-"}
        </td>
        <td className="px-4 py-2">
          <Badge className={`text-xs ${statusBadgeStyle(inv.status)}`}>
            {inv.status}
          </Badge>
        </td>
      </tr>
    ));
  };

  return (
    <div className="overflow-x-auto">
      {error && (
        <div className="mb-4 p-3 bg-red-50 text-red-800 rounded-md text-sm">{error}</div>
      )}
      <table className="min-w-full divide-y divide-gray-200">
        <thead className="bg-gray-50">
          <tr>
            {columns.map((col) => (
              <SortableHeader
                key={col}
                column={col}
                label={columnLabel(col)}
                sort={sort}
                onClick={handleHeaderClick}
              />
            ))}
          </tr>
        </thead>
        <tbody className="bg-white">{renderRows()}</tbody>
      </table>
    </div>
  );
};

export default InvoiceTable;