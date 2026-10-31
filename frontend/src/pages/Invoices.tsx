import React, { useState, useEffect, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import { PlusIcon, TrashIcon, CheckCircleIcon, FunnelIcon } from '@heroicons/react/24/outline';
import { useInvoices } from '@/hooks/useInvoices';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { client } from '@/src/api/client';
import InvoiceTable from '@/src/components/InvoiceTable';
import Modal from '@/src/components/Modal';
import Toast from '@/src/components/Toast';
import { format } from 'date-fns';
import { debounce } from 'lodash';

// Types (adjust if a global type definition exists)
interface Invoice {
  id: string;
  vendor: string;
  invoice_number: string;
  date: string; // ISO string
  total: number;
  status: 'pending' | 'processing' | 'completed' | 'failed';
}

// Helper to map status to badge styles
const statusBadge = (status: Invoice['status']) => {
  const base = 'px-2 py-0.5 rounded text-xs font-medium';
  switch (status) {
    case 'pending':
      return `${base} bg-gray-100 text-gray-800`;
    case 'processing':
      return `${base} bg-blue-100 text-blue-800`;
    case 'completed':
      return `${base} bg-green-100 text-green-800`;
    case 'failed':
      return `${base} bg-red-100 text-red-800`;
    default:
      return base;
  }
};

const Invoices: React.FC = () => {
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  // Filter state
  const [vendorSearch, setVendorSearch] = useState('');
  const [statusFilter, setStatusFilter] = useState<string>('');
  const [dateFrom, setDateFrom] = useState<string>('');
  const [dateTo, setDateTo] = useState<string>('');

  // Bulk selection
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [showDeleteModal, setShowDeleteModal] = useState(false);

  // Toast handling
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);

  // Load invoices with custom hook
  const {
    invoices,
    total,
    page,
    pageSize,
    setPage,
    setPageSize,
    isLoading,
    isError,
    error,
    refetch,
  } = useInvoices({
    vendor: vendorSearch,
    status: statusFilter,
    dateFrom,
    dateTo,
    page,
    pageSize,
  });

  // Debounce vendor search to avoid excessive queries
  const debouncedSetVendor = useMemo(
    () => debounce((value: string) => setVendorSearch(value), 300),
    []
  );

  // Effect to reset selection when data changes
  useEffect(() => {
    setSelectedIds(new Set());
  }, [invoices]);

  // Bulk delete mutation
  const deleteMutation = useMutation(
    async (ids: string[]) => {
      await client.delete('/api/invoices/bulk', { data: { ids } });
    },
    {
      onSuccess: () => {
        setToast({ message: 'Invoices deleted successfully.', type: 'success' });
        queryClient.invalidateQueries(['invoices']);
        setShowDeleteModal(false);
      },
      onError: () => {
        setToast({ message: 'Failed to delete invoices.', type: 'error' });
      },
    }
  );

  // Bulk mark as reviewed mutation (example action)
  const markReviewedMutation = useMutation(
    async (ids: string[]) => {
      await client.patch('/api/invoices/bulk', { ids, action: 'mark_reviewed' });
    },
    {
      onSuccess: () => {
        setToast({ message: 'Invoices marked as reviewed.', type: 'success' });
        queryClient.invalidateQueries(['invoices']);
      },
      onError: () => {
        setToast({ message: 'Failed to update invoices.', type: 'error' });
      },
    }
  );

  const handleSelectAll = (checked: boolean) => {
    if (checked) {
      const allIds = invoices.map((inv) => inv.id);
      setSelectedIds(new Set(allIds));
    } else {
      setSelectedIds(new Set());
    }
  };

  const handleSelectRow = (id: string, checked: boolean) => {
    setSelectedIds((prev) => {
      const newSet = new Set(prev);
      if (checked) {
        newSet.add(id);
      } else {
        newSet.delete(id);
      }
      return newSet;
    });
  };

  const handleBulkDelete = () => {
    if (selectedIds.size === 0) return;
    setShowDeleteModal(true);
  };

  const confirmBulkDelete = () => {
    deleteMutation.mutate(Array.from(selectedIds));
  };

  const handleBulkMarkReviewed = () => {
    if (selectedIds.size === 0) return;
    markReviewedMutation.mutate(Array.from(selectedIds));
  };

  const renderBulkActions = () => (
    <div className="flex items-center space-x-3">
      <button
        type="button"
        onClick={handleBulkMarkReviewed}
        disabled={selectedIds.size === 0 || markReviewedMutation.isLoading}
        className="inline-flex items-center rounded-md border border-transparent bg-emerald-600 px-3 py-2 text-sm font-medium text-white shadow-sm hover:bg-emerald-700 focus:outline-none focus:ring-2 focus:ring-emerald-500 focus:ring-offset-2 disabled:opacity-50"
      >
        <CheckCircleIcon className="h-4 w-4 mr-1" aria-hidden="true" />
        Mark Reviewed
      </button>
      <button
        type="button"
        onClick={handleBulkDelete}
        disabled={selectedIds.size === 0 || deleteMutation.isLoading}
        className="inline-flex items-center rounded-md border border-transparent bg-red-600 px-3 py-2 text-sm font-medium text-white shadow-sm hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-red-500 focus:ring-offset-2 disabled:opacity-50"
      >
        <TrashIcon className="h-4 w-4 mr-1" aria-hidden="true" />
        Delete
      </button>
    </div>
  );

  const renderFilters = () => (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
      <div>
        <label htmlFor="vendor" className="block text-sm font-medium text-gray-700">
          Vendor
        </label>
        <input
          id="vendor"
          type="text"
          placeholder="Search vendor"
          className="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-primary-500 focus:ring-primary-500 sm:text-sm"
          onChange={(e) => debouncedSetVendor(e.target.value.trim())}
        />
      </div>

      <div>
        <label htmlFor="status" className="block text-sm font-medium text-gray-700">
          Status
        </label>
        <select
          id="status"
          value={statusFilter}
          onChange={(e) => setStatusFilter(e.target.value)}
          className="mt-1 block w-full rounded-md border-gray-300 bg-white py-2 pl-3 pr-10 shadow-sm focus:border-primary-500 focus:outline-none focus:ring-primary-500 sm:text-sm"
        >
          <option value="">All</option>
          <option value="pending">Pending</option>
          <option value="processing">Processing</option>
          <option value="completed">Completed</option>
          <option value="failed">Failed</option>
        </select>
      </div>

      <div>
        <label htmlFor="dateFrom" className="block text-sm font-medium text-gray-700">
          Date From
        </label>
        <input
          id="dateFrom"
          type="date"
          value={dateFrom}
          onChange={(e) => setDateFrom(e.target.value)}
          className="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-primary-500 focus:ring-primary-500 sm:text-sm"
        />
      </div>

      <div>
        <label htmlFor="dateTo" className="block text-sm font-medium text-gray-700">
          Date To
        </label>
        <input
          id="dateTo"
          type="date"
          value={dateTo}
          onChange={(e) => setDateTo(e.target.value)}
          className="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-primary-500 focus:ring-primary-500 sm:text-sm"
        />
      </div>
    </div>
  );

  const renderPagination = () => {
    const totalPages = Math.ceil(total / pageSize);
    const pages = [];

    for (let i = 1; i <= totalPages; i++) {
      pages.push(
        <button
          key={i}
          onClick={() => setPage(i)}
          disabled={i === page}
          className={`px-3 py-1 rounded-md ${
            i === page
              ? 'bg-primary-600 text-white'
              : 'bg-white text-gray-700 hover:bg-gray-100'
          }`}
        >
          {i}
        </button>
      );
    }

    return (
      <nav className="flex items-center space-x-2" aria-label="Pagination">
        <button
          onClick={() => setPage(Math.max(page - 1, 1))}
          disabled={page === 1}
          className="px-2 py-1 rounded-md bg-white text-gray-700 hover:bg-gray-100 disabled:opacity-50"
        >
          Previous
        </button>
        {pages}
        <button
          onClick={() => setPage(Math.min(page + 1, totalPages))}
          disabled={page === totalPages}
          className="px-2 py-1 rounded-md bg-white text-gray-700 hover:bg-gray-100 disabled:opacity-50"
        >
          Next
        </button>
        <select
          value={pageSize}
          onChange={(e) => setPageSize(Number(e.target.value))}
          className="ml-4 rounded-md border-gray-300 text-sm"
        >
          {[10, 25, 50, 100].map((size) => (
            <option key={size} value={size}>
              {size} / page
            </option>
          ))}
        </select>
      </nav>
    );
  };

  if (isLoading) {
    return (
      <div className="flex justify-center items-center h-64">
        <svg className="animate-spin h-8 w-8 text-primary-600" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
          <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
          <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"></path>
        </svg>
      </div>
    );
  }

  if (isError) {
    return (
      <div className="p-6">
        <p className="text-red-600">Error loading invoices: {(error as Error).message}</p>
        <button
          onClick={() => refetch()}
          className="mt-4 inline-flex items-center rounded-md border border-transparent bg-primary-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
        >
          Retry
        </button>
      </div>
    );
  }

  return (
    <div className="p-6 bg-gray-50 min-h-screen">
      {/* Header */}
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-semibold text-gray-900">Invoices</h1>
        <button
          type="button"
          onClick={() => navigate('/invoices/upload')}
          className="inline-flex items-center rounded-md border border-transparent bg-primary-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
        >
          <PlusIcon className="h-5 w-5 mr-2" aria-hidden="true" />
          New Invoice
        </button>
      </div>

      {/* Filters */}
      <section className="mb-6 rounded-lg bg-white p-4 shadow-sm">{renderFilters()}</section>

      {/* Bulk actions */}
      {selectedIds.size > 0 && (
        <section className="mb-4 flex items-center justify-between rounded bg-primary-50 p-3">
          <p className="text-sm text-primary-800">{selectedIds.size} selected</p>
          {renderBulkActions()}
        </section>
      )}

      {/* Table */}
      <section className="overflow-x-auto rounded bg-white shadow-sm">
        <InvoiceTable
          invoices={invoices}
          selectedIds={selectedIds}
          onSelectAll={handleSelectAll}
          onSelectRow={handleSelectRow}
          renderStatusBadge={(status) => <span className={statusBadge(status)}>{status}</span>}
        />
      </section>

      {/* Pagination */}
      <div className="mt-4 flex items-center justify-between">{renderPagination()}</div>

      {/* Delete Confirmation Modal */}
      <Modal
        isOpen={showDeleteModal}
        title="Confirm Deletion"
        onClose={() => setShowDeleteModal(false)}
        onConfirm={confirmBulkDelete}
        confirmLabel="Delete"
        confirmVariant="danger"
      >
        <p className="text-sm text-gray-600">
          Are you sure you want to delete the selected {selectedIds.size} invoice
          {selectedIds.size > 1 ? 's' : ''}? This action cannot be undone.
        </p>
      </Modal>

      {/* Toast notifications */}
      {toast && (
        <Toast
          type={toast.type}
          onClose={() => setToast(null)}
          className="fixed bottom-4 right-4 max-w-xs"
        >
          {toast.message}
        </Toast>
      )}
    </div>
  );
};

export default Invoices;