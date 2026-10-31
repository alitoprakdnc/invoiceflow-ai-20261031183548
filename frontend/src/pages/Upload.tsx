import React, { useState, useCallback, useEffect } from "react";
import { useNavigate } from "react-router-dom";
import { AxiosProgressEvent } from "axios";
import client from "@/api/client";
import UploadDropzone from "@/components/UploadDropzone";
import Toast from "@/components/Toast";

interface UploadFile {
  file: File;
  previewUrl: string;
  progress: number; // 0‑100
  uploaded: boolean;
  error?: string;
}

/**
 * Upload page – allows users to drag‑and‑drop one or more invoices,
 * shows a preview, per‑file progress bar and reports success / error
 * via toast notifications.
 */
const Upload: React.FC = () => {
  const [files, setFiles] = useState<UploadFile[]>([]);
  const [isUploading, setIsUploading] = useState(false);
  const [toasts, setToasts] = useState<
    { id: number; type: "success" | "error"; message: string }[]
  >([]);
  const navigate = useNavigate();

  // Cleanup object URLs when component unmounts or files change
  useEffect(() => {
    return () => {
      files.forEach((f) => URL.revokeObjectURL(f.previewUrl));
    };
  }, [files]);

  const addToast = useCallback(
    (type: "success" | "error", message: string) => {
      const id = Date.now();
      setToasts((prev) => [...prev, { id, type, message }]);
      setTimeout(() => {
        setToasts((prev) => prev.filter((t) => t.id !== id));
      }, 5000);
    },
    []
  );

  const handleDrop = useCallback((acceptedFiles: File[]) => {
    const newFiles: UploadFile[] = acceptedFiles.map((file) => ({
      file,
      previewUrl: URL.createObjectURL(file),
      progress: 0,
      uploaded: false,
    }));
    setFiles((prev) => [...prev, ...newFiles]);
  }, []);

  const handleRemove = useCallback((index: number) => {
    setFiles((prev) => {
      const updated = [...prev];
      const [removed] = updated.splice(index, 1);
      URL.revokeObjectURL(removed.previewUrl);
      return updated;
    });
  }, []);

  const uploadSingleFile = async (uploadFile: UploadFile, idx: number) => {
    const formData = new FormData();
    formData.append("file", uploadFile.file);

    try {
      await client.post("/api/invoices/upload", formData, {
        headers: {
          "Content-Type": "multipart/form-data",
        },
        onUploadProgress: (progressEvent: AxiosProgressEvent) => {
          const percent = Math.round(
            (progressEvent.loaded * 100) / (progressEvent.total ?? 1)
          );
          setFiles((prev) => {
            const copy = [...prev];
            copy[idx] = { ...copy[idx], progress: percent };
            return copy;
          });
        },
      });

      setFiles((prev) => {
        const copy = [...prev];
        copy[idx] = { ...copy[idx], uploaded: true, progress: 100 };
        return copy;
      });
      addToast("success", `${uploadFile.file.name} uploaded successfully`);
    } catch (err: any) {
      const message =
        err.response?.data?.error ?? err.message ?? "Upload failed";
      setFiles((prev) => {
        const copy = [...prev];
        copy[idx] = { ...copy[idx], error: message };
        return copy;
      });
      addToast("error", `${uploadFile.file.name}: ${message}`);
    }
  };

  const handleUpload = async () => {
    if (files.length === 0) return;
    setIsUploading(true);
    for (let i = 0; i < files.length; i++) {
      // Skip already uploaded files
      if (files[i].uploaded) continue;
      await uploadSingleFile(files[i], i);
    }
    setIsUploading(false);
    // Optionally navigate to invoices list after successful upload
    const allSuccess = files.every((f) => f.uploaded && !f.error);
    if (allSuccess) {
      navigate("/invoices");
    }
  };

  return (
    <div className="max-w-4xl mx-auto p-6">
      {/* Toast container */}
      <div className="fixed top-4 right-4 z-50 flex flex-col gap-2">
        {toasts.map((t) => (
          <Toast
            key={t.id}
            type={t.type}
            onClose={() =>
              setToasts((prev) => prev.filter((toast) => toast.id !== t.id))
            }
          >
            {t.message}
          </Toast>
        ))}
      </div>

      <h1 className="text-2xl font-semibold text-gray-900 dark:text-gray-100 mb-4">
        Upload Invoices
      </h1>

      <UploadDropzone onDrop={handleDrop} disabled={isUploading} />

      {files.length > 0 && (
        <section className="mt-6 space-y-4">
          <h2 className="text-lg font-medium text-gray-800 dark:text-gray-200">
            Selected Files
          </h2>
          <ul className="divide-y divide-gray-200 dark:divide-gray-700">
            {files.map((f, idx) => (
              <li
                key={idx}
                className="flex items-center py-3 md:py-4 md:space-x-6"
              >
                {/* Preview */}
                <div className="flex-shrink-0 w-16 h-16 bg-gray-100 dark:bg-gray-800 rounded-md overflow-hidden">
                  {f.file.type.startsWith("image/") ? (
                    <img
                      src={f.previewUrl}
                      alt={f.file.name}
                      className="object-cover w-full h-full"
                    />
                  ) : (
                    <div className="flex items-center justify-center h-full text-gray-500 dark:text-gray-400">
                      <svg
                        className="w-8 h-8"
                        fill="none"
                        stroke="currentColor"
                        viewBox="0 0 24 24"
                      >
                        <path
                          strokeLinecap="round"
                          strokeLinejoin="round"
                          strokeWidth={2}
                          d="M12 4v16m8-8H4"
                        />
                      </svg>
                    </div>
                  )}
                </div>

                {/* File info */}
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium text-gray-900 dark:text-gray-100 truncate">
                    {f.file.name}
                  </p>
                  <p className="text-sm text-gray-500 dark:text-gray-400">
                    {Math.round(f.file.size / 1024)} KB
                  </p>

                  {/* Progress bar */}
                  <div className="mt-2 w-full bg-gray-200 dark:bg-gray-700 rounded-full h-2.5">
                    <div
                      className={`h-2.5 rounded-full transition-all ${
                        f.error
                          ? "bg-red-600"
                          : f.uploaded
                          ? "bg-green-600"
                          : "bg-primary-600"
                      }`}
                      style={{ width: `${f.progress}%` }}
                    />
                  </div>

                  {f.error && (
                    <p className="mt-1 text-sm text-red-600">{f.error}</p>
                  )}
                </div>

                {/* Remove button */}
                <div className="flex-shrink-0">
                  <button
                    type="button"
                    onClick={() => handleRemove(idx)}
                    disabled={isUploading}
                    className="inline-flex items-center p-2 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200 focus:outline-none focus:ring-2 focus:ring-primary-500 rounded"
                    aria-label={`Remove ${f.file.name}`}
                  >
                    <svg
                      className="w-5 h-5"
                      fill="none"
                      stroke="currentColor"
                      viewBox="0 0 24 24"
                    >
                      <path
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth={2}
                        d="M6 18L18 6M6 6l12 12"
                      />
                    </svg>
                  </button>
                </div>
              </li>
            ))}
          </ul>

          <div className="flex justify-end">
            <button
              type="button"
              onClick={handleUpload}
              disabled={isUploading || files.every((f) => f.uploaded)}
              className={`px-5 py-2.5 text-sm font-medium text-white bg-primary-600 rounded-md hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-primary-500 disabled:opacity-50 disabled:cursor-not-allowed transition-colors`}
            >
              {isUploading ? "Uploading…" : "Upload"}
            </button>
          </div>
        </section>
      )}
    </div>
  );
};

export default Upload;