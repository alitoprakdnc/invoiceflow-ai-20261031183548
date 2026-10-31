import { useState } from 'react';
import {
  useQuery,
  useMutation,
  useQueryClient,
  UseQueryResult,
  UseMutationResult,
} from '@tanstack/react-query';
import client from '@/api/client';

/**
 * Invoice entity returned by the backend.
 */
export interface Invoice {
  id: string;
  vendor: string;
  invoice_number: string;
  date: string; // ISO‑8601
  total: number;
  status: 'pending' | 'processed' | 'error';
  created_at: string;
  updated_at: string;
  // Extend with additional fields as needed
}

/**
 * Filters that can be applied when listing invoices.
 */
export interface InvoiceFilters {
  vendor?: string;
  status?: 'pending' | 'processed' | 'error';
  dateFrom?: string; // ISO‑8601
  dateTo?: string;   // ISO‑8601
  minTotal?: number;
  maxTotal?: number;
}

/**
 * Shape of the paginated response from `/api/invoices`.
 */
export interface PaginatedInvoicesResponse {
  invoices: Invoice[];
  total: number;
  page: number;
  limit: number;
}

/**
 * Hook that encapsulates all invoice‑related data fetching and mutations.
 *
 * It provides:
 *  - Paginated, filterable query of invoices.
 *  - Mutations for create, update and delete with optimistic cache updates.
 *  - Helpers to change pagination / filter state which automatically trigger a refetch.
 *
 * @param initialPage   Starting page number (1‑based).
 * @param initialLimit Number of items per page.
 * @param initialFilters Optional initial filter set.
 */
export const useInvoices = (
  initialPage = 1,
  initialLimit = 20,
  initialFilters: InvoiceFilters = {}
) => {
  // -------------------------------------------------------------------------
  // Local UI state (pagination + filters)
  // -------------------------------------------------------------------------
  const [page, setPage] = useState<number>(initialPage);
  const [limit, setLimit] = useState<number>(initialLimit);
  const [filters, setFilters] = useState<InvoiceFilters>(initialFilters);

  // -------------------------------------------------------------------------
  // React‑Query: fetch invoices
  // -------------------------------------------------------------------------
  const queryKey = ['invoices', { page, limit, filters }];

  const fetchInvoices = async (): Promise<PaginatedInvoicesResponse> => {
    const params: Record<string, unknown> = {
      page,
      limit,
      ...filters,
    };

    // Remove undefined values – the backend treats missing params as “no filter”.
    Object.keys(params).forEach(
      (k) => params[k] === undefined && delete params[k]
    );

    const { data } = await client.get<PaginatedInvoicesResponse>(
      '/api/invoices',
      { params }
    );
    return data;
  };

  const {
    data,
    isLoading,
    isError,
    error,
    refetch,
  }: UseQueryResult<PaginatedInvoicesResponse, Error> = useQuery<
    PaginatedInvoicesResponse,
    Error
  >(queryKey, fetchInvoices, {
    keepPreviousData: true, // smooth UI when paging
    staleTime: 60_000, // 1 min
    retry: 2,
  });

  // -------------------------------------------------------------------------
  // Mutations (create / update / delete) with cache synchronization
  // -------------------------------------------------------------------------
  const queryClient = useQueryClient();

  const createMutation: UseMutationResult<Invoice, Error, Partial<Invoice>> =
    useMutation(
      (newInvoice) =>
        client
          .post<Invoice>('/api/invoices', newInvoice)
          .then((res) => res.data),
      {
        onSuccess: (created) => {
          // Prepend the newly created invoice to the current page cache.
          queryClient.setQueryData<PaginatedInvoicesResponse>(queryKey, (old) => {
            if (!old) {
              return {
                invoices: [created],
                total: 1,
                page,
                limit,
              };
            }
            return {
              ...old,
              invoices: [created, ...old.invoices],
              total: old.total + 1,
            };
          });
        },
        onError: (err) => {
          console.error('Create invoice failed:', err);
        },
      }
    );

  const updateMutation: UseMutationResult<
    Invoice,
    Error,
    { id: string; data: Partial<Invoice> }
  > = useMutation(
    ({ id, data }) =>
      client
        .patch<Invoice>(`/api/invoices/${id}`, data)
        .then((res) => res.data),
    {
      onSuccess: (updated) => {
        queryClient.setQueryData<PaginatedInvoicesResponse>(queryKey, (old) => {
          if (!old) return old;
          const idx = old.invoices.findIndex((i) => i.id === updated.id);
          if (idx === -1) return old;
          const newInvoices = [...old.invoices];
          newInvoices[idx] = updated;
          return { ...old, invoices: newInvoices };
        });
      },
      onError: (err) => {
        console.error('Update invoice failed:', err);
      },
    }
  );

  const deleteMutation: UseMutationResult<void, Error, string> = useMutation(
    (id) => client.delete<void>(`/api/invoices/${id}`).then((res) => res.data),
    {
      onSuccess: (_, id) => {
        queryClient.setQueryData<PaginatedInvoicesResponse>(queryKey, (old) => {
          if (!old) return old;
          const newInvoices = old.invoices.filter((i) => i.id !== id);
          return {
            ...old,
            invoices: newInvoices,
            total: old.total - 1,
          };
        });
      },
      onError: (err) => {
        console.error('Delete invoice failed:', err);
      },
    }
  );

  // -------------------------------------------------------------------------
  // Return API for components
  // -------------------------------------------------------------------------
  return {
    // Data
    invoices: data?.invoices ?? [],
    total: data?.total ?? 0,
    page,
    limit,
    isLoading,
    isError,
    error,
    // Pagination / filter helpers
    setPage,
    setLimit,
    setFilters,
    // Manual refetch (e.g., after a background webhook update)
    refetch,
    // Mutations
    createInvoice: createMutation.mutate,
    createInvoiceStatus: createMutation.status,
    updateInvoice: updateMutation.mutate,
    updateInvoiceStatus: updateMutation.status,
    deleteInvoice: deleteMutation.mutate,
    deleteInvoiceStatus: deleteMutation.status,
  };
};