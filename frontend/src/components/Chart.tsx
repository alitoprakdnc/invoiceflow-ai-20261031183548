import React, { useMemo } from "react";
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  BarElement,
  LineElement,
  PointElement,
  Title,
  Tooltip,
  Legend,
  ChartData,
  ChartOptions,
} from "chart.js";
import { Chart as ReactChart } from "react-chartjs-2";

ChartJS.register(
  CategoryScale,
  LinearScale,
  BarElement,
  LineElement,
  PointElement,
  Title,
  Tooltip,
  Legend
);

export type ChartType = "line" | "bar";

export interface ChartProps {
  /** Type of chart to render – line or bar */
  type: ChartType;
  /** Data object compatible with Chart.js */
  data: ChartData<ChartType>;
  /** Optional Chart.js options – merged with sensible defaults */
  options?: ChartOptions<ChartType>;
  /** Optional title displayed above the chart (also used for ARIA label) */
  title?: string;
  /** Additional Tailwind classes for the outer container */
  className?: string;
}

/**
 * Reusable Chart component built on top of Chart.js via react-chartjs-2.
 *
 * - Responsive, maintains aspect ratio based on container size.
 * - Accessible: the container has `role="img"` and an `aria-label` derived from the title.
 * - Tailwind‑styled card with light/dark mode support.
 * - Merges user‑provided options with sensible defaults (legend, tooltip, title).
 */
export const Chart: React.FC<ChartProps> = ({
  type,
  data,
  options,
  title,
  className = "",
}) => {
  // Memoize merged options to avoid unnecessary re‑renders
  const mergedOptions = useMemo<ChartOptions<ChartType>>(() => {
    const base: ChartOptions<ChartType> = {
      responsive: true,
      maintainAspectRatio: false,
      plugins: {
        legend: {
          position: "top",
          labels: {
            // Use Tailwind colors that adapt to dark mode
            color: "inherit",
          },
        },
        title: {
          display: !!title,
          text: title,
          color: "inherit",
          font: {
            size: 16,
            weight: "600",
          },
          padding: {
            top: 10,
            bottom: 30,
          },
        },
        tooltip: {
          enabled: true,
          mode: "index",
          intersect: false,
        },
      },
      scales: {
        x: {
          grid: {
            display: false,
          },
          ticks: {
            color: "inherit",
          },
        },
        y: {
          grid: {
            drawBorder: false,
          },
          ticks: {
            color: "inherit",
          },
        },
      },
    };

    // Deep merge user options over the base defaults
    return {
      ...base,
      ...options,
      plugins: {
        ...base.plugins,
        ...(options?.plugins ?? {}),
      },
      scales: {
        ...base.scales,
        ...(options?.scales ?? {}),
      },
    };
  }, [options, title]);

  // Choose the correct component based on chart type
  const ChartComponent = type === "line" ? ReactChart : ReactChart;

  return (
    <div
      className={`bg-white dark:bg-gray-800 p-4 rounded-lg shadow-sm ${className}`}
      role="img"
      aria-label={title ?? "Chart"}
    >
      <div className="h-64 w-full">
        <ChartComponent
          type={type}
          data={data}
          options={mergedOptions}
          // react-chartjs-2 v4 uses `plugins` prop for custom plugins; we don't need extra here
        />
      </div>
    </div>
  );
};

export default Chart;