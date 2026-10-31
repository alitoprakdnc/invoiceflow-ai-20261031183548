import React, { useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { apiClient } from "@/api/client";
import { Chart } from "@/components/Chart";
import { InvoiceTable } from "@/components/InvoiceTable";
import { Toast } from "@/components/Toast";
import { format, parseISO } from "date-fns";

type ProcessedPerDay = {
  date: string; // ISO date string (YYYY-MM-DD)
  count: number;
};

type DashboardData = {
  processedPerDay: ProcessedPerDay[];
  errorRate: number; // 0‑1
  webhookSuccessRate: number; // 0‑1
  recentInvoices: Invoice[];
};

type Invoice = {
  id: string;
  vendor: string;
  total: number;
  status: "pending" | "processed" | "failed";
  created_at: string; // ISO timestamp
};

const fetchDashboard = async (): Promise<DashboardData> => {
  const { data } = await apiClient.get<DashboardData>("/api/dashboard");
  return data;
};

export const Dashboard: React.FC = () => {
  const {
    data,
    isLoading,
    isError,
    error,
    refetch,
  } = useQuery<DashboardData, Error>(["dashboard"], fetchDashboard, {
    staleTime: 60_000, // 1 minute
    refetchInterval: 30_000, // keep UI fresh
  });

  // Show a toast on error (once)
  useEffect(() => {
    if (isError && error) {
      Toast.error(
        "Failed to load dashboard data",
        error.message ?? "An unexpected error occurred"
      );
    }
  }, [isError, error]);

  // Prepare chart data
  const chartLabels = data?.processedPerDay.map((d) =>
    format(parseISO(d.date), "MMM d")
  );
  const chartValues = data?.processedPerDay.map((d) => d.count);

  const chartData = {
    labels: chartLabels ?? [],
    datasets: [
      {
        label: "Invoices processed",
        data: chartValues ?? [],
        backgroundColor: "rgba(10, 116, 218, 0.6)",
        borderColor: "rgba(10, 116, 218, 1)",
        borderWidth: 1,
        tension: 0.3,
      },
    ],
  };

  const chartOptions = {
    responsive: true,
    maintainAspectRatio: false,
    plugins: {
      legend: {
        display: false,
      },
    },
    scales: {
      y: {
        beginAtZero: true,
        ticks: {
          stepSize: 1,
        },
      },
    },
  };

  return (
    <main className="p-6 bg-gray-50 dark:bg-gray-900 min-h-screen">
      <header className="mb-8">
        <h1 className="text-2xl font-semibold text-gray-900 dark:text-gray-100">
          Dashboard
        </h1>
        <p className="mt-1 text-sm text-gray-600 dark:text-gray-400">
          Overview of invoice processing and system health.
        </p>
      </header>

      {/* Stats cards */}
      <section className="grid grid-cols-1 md:grid-cols-3 gap-6 mb-8">
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow-sm p-6 flex flex-col">
          <h2 className="text-lg font-medium text-gray-800 dark:text-gray-200">
            Total processed today
          </h2>
          <p className="mt-2 text-3xl font-bold text-primary-600">
            {data?.processedPerDay?.[data.processedPerDay.length - 1]?.count ??
              "-"}
          </p>
        </div>

        <div className="bg-white dark:bg-gray-800 rounded-lg shadow-sm p-6 flex flex-col">
          <h2 className="text-lg font-medium text-gray-800 dark:text-gray-200">
            Error rate
          </h2>
          <p className="mt-2 text-3xl font-bold text-amber-600">
            {data ? `${(data.errorRate * 100).toFixed(1)}%` : "-"}
          </p>
        </div>

        <div className="bg-white dark:bg-gray-800 rounded-lg shadow-sm p-6 flex flex-col">
          <h2 className="text-lg font-medium text-gray-800 dark:text-gray-200">
            Webhook success
          </h2>
          <p className="mt-2 text-3xl font-bold text-emerald-600">
            {data ? `${(data.webhookSuccessRate * 100).toFixed(1)}%` : "-"}
          </p>
        </div>
      </section>

      {/* Charts */}
      <section className="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-8">
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow-sm p-4 h-80">
          <h2 className="text-lg font-medium text-gray-800 dark:text-gray-200 mb-2">
            Invoices processed per day
          </h2>
          <div className="h-full">
            <Chart type="line" data={chartData} options={chartOptions} />
          </div>
        </div>

        <div className="bg-white dark:bg-gray-800 rounded-lg shadow-sm p-4 h-80 flex flex-col">
          <h2 className="text-lg font-medium text-gray-800 dark:text-gray-200 mb-2">
            Recent activity
          </h2>
          {isLoading ? (
            <div className="flex-1 flex items-center justify-center">
              <svg
                className="animate-spin h-8 w-8 text-primary-600"
                xmlns="http://www.w3.org/2000/svg"
                fill="none"
                viewBox="0 0 24 24"
                aria-label="Loading"
              >
                <circle
                  className="opacity-25"
                  cx="12"
                  cy="12"
                  r="10"
                  stroke="currentColor"
                  strokeWidth="4"
                />
                <path
                  className="opacity-75"
                  fill="currentColor"
                  d="M4 12a8 8 0 018-8v8H4z"
                />
              </svg>
            </div>
          ) : (
            <InvoiceTable invoices={data?.recentInvoices ?? []} />
          )}
        </div>
      </section>

      {/* Refresh button */}
      <div className="flex justify-end">
        <button
          type="button"
          onClick={() => refetch()}
          disabled={isLoading}
          className="inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md shadow-sm text-white bg-primary-600 hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500 disabled:opacity-50"
        >
          {isLoading ? (
            <svg
              className="animate-spin -ml-1 mr-2 h-5 w-5 text-white"
              xmlns="http://www.w3.org/2000/svg"
              fill="none"
              viewBox="0 0 24 24"
              aria-hidden="true"
            >
              <circle
                className="opacity-25"
                cx="12"
                cy="12"
                r="10"
                stroke="currentColor"
                strokeWidth="4"
              />
              <path
                className="opacity-75"
                fill="currentColor"
                d="M4 12a8 8 0 018-8v8H4z"
              />
            </svg>
          ) : null}
          Refresh
        </button>
      </div>
    </main>
  );
};

export default Dashboard;