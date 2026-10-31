import React, { useCallback, useEffect, useState } from "react";
import { useDropzone, FileRejection } from "react-dropzone";

interface UploadDropzoneProps {
  /** Called with the list of files that passed validation */
  onUpload: (files: File[]) => Promise<void> | void;
  /** MIME types accepted – defaults to PDFs and common image types */
  accept?: { [mime: string]: string[] };
  /** Maximum file size in bytes – defaults to 10 MiB */
  maxSize?: number;
  /** Allow multiple file selection – defaults to true */
  multiple?: boolean;
  /** Maximum number of files that can be selected – defaults to 10 */
  maxFiles?: number;
}

/**
 * Dropzone component built with react-dropzone.
 * - Validates file type and size.
 * - Shows thumbnail previews for images, generic icons for PDFs.
 * - Accessible keyboard navigation and ARIA labels.
 * - Calls `onUpload` with the accepted files.
 */
const UploadDropzone: React.FC<UploadDropzoneProps> = ({
  onUpload,
  accept = {
    "application/pdf": [".pdf"],
    "image/jpeg": [".jpeg", ".jpg"],
    "image/png": [".png"],
    "image/webp": [".webp"],
  },
  maxSize = 10 * 1024 * 1024, // 10 MiB
  multiple = true,
  maxFiles = 10,
}) => {
  const [files, setFiles] = useState<File[]>([]);
  const [previews, setPreviews] = useState<{ file: File; url: string }[]>([]);
  const [errorMessages, setErrorMessages] = useState<string[]>([]);

  // Generate object URLs for image previews
  useEffect(() => {
    const imageFiles = files.filter((f) => f.type.startsWith("image/"));
    const newPreviews = imageFiles.map((file) => ({
      file,
      url: URL.createObjectURL(file),
    }));
    setPreviews(newPreviews);

    // Revoke URLs on cleanup
    return () => {
      newPreviews.forEach((p) => URL.revokeObjectURL(p.url));
    };
  }, [files]);

  // Call the parent upload handler whenever the accepted file list changes
  useEffect(() => {
    if (files.length > 0) {
      void onUpload(files);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [files]);

  const onDrop = useCallback(
    (accepted: File[], rejections: FileRejection[]) => {
      const newErrors: string[] = [];

      // Process rejections
      rejections.forEach((rej) => {
        const msgs = rej.errors.map((e) => e.message).join(", ");
        newErrors.push(`"${rej.file.name}": ${msgs}`);
      });

      // Enforce maxFiles limit
      const totalAfterDrop = files.length + accepted.length;
      if (totalAfterDrop > maxFiles) {
        const allowed = maxFiles - files.length;
        newErrors.push(
          `You can only add ${allowed} more file${
            allowed !== 1 ? "s" : ""
          } (limit of ${maxFiles})`
        );
        // Trim accepted files to the allowed number
        accepted = accepted.slice(0, allowed);
      }

      if (accepted.length) {
        setFiles((prev) => [...prev, ...accepted]);
      }
      setErrorMessages(newErrors);
    },
    [files, maxFiles]
  );

  const {
    getRootProps,
    getInputProps,
    isDragActive,
    isDragReject,
    isFocused,
  } = useDropzone({
    onDrop,
    accept,
    maxSize,
    multiple,
  });

  const removeFile = (file: File) => {
    setFiles((curr) => curr.filter((f) => f !== file));
    setErrorMessages([]);
  };

  return (
    <section className="w-full">
      {/* Dropzone area */}
      <div
        {...getRootProps()}
        className={`
          flex flex-col items-center justify-center p-6 border-2 border-dashed rounded-lg
          transition-colors cursor-pointer
          ${isDragActive ? "border-primary-600 bg-primary-50 dark:bg-primary-900/30" : ""}
          ${isDragReject ? "border-red-600 bg-red-50 dark:bg-red-900/30" : ""}
          ${isFocused ? "ring-2 ring-primary-500" : ""}
          bg-white dark:bg-gray-800
        `}
        aria-label="File upload dropzone"
      >
        <input {...getInputProps()} aria-describedby="dropzone-help" />
        <svg
          className="w-12 h-12 text-gray-400 dark:text-gray-500 mb-3"
          xmlns="http://www.w3.org/2000/svg"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
          aria-hidden="true"
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            strokeWidth={2}
            d="M7 16V4h10v12M7 20h10M9 8h6"
          />
        </svg>
        <p className="text-sm text-gray-600 dark:text-gray-300">
          {isDragActive
            ? "Drop the files here …"
            : "Drag & drop files here, or click to select"}
        </p>
        <p id="dropzone-help" className="mt-1 text-xs text-gray-500 dark:text-gray-400">
          Accepted: PDF, JPEG, PNG, WebP – up to {Math.round(maxSize / (1024 * 1024))} MiB each
        </p>
      </div>

      {/* Error messages */}
      {errorMessages.length > 0 && (
        <ul className="mt-4 space-y-1 text-sm text-red-600 dark:text-red-400">
          {errorMessages.map((msg, idx) => (
            <li key={idx}>⚠️ {msg}</li>
          ))}
        </ul>
      )}

      {/* Preview list */}
      {files.length > 0 && (
        <aside className="mt-6">
          <h2 className="text-sm font-medium text-gray-700 dark:text-gray-200 mb-2">
            Selected file{files.length > 1 ? "s" : ""}
          </h2>
          <ul className="grid grid-cols-1 gap-4 sm:grid-cols-2 md:grid-cols-3">
            {files.map((file) => {
              const preview = previews.find((p) => p.file === file);
              const isImage = file.type.startsWith("image/");
              return (
                <li
                  key={file.name + file.size}
                  className="flex items-center space-x-3 p-3 border rounded-lg bg-gray-50 dark:bg-gray-700"
                >
                  {isImage && preview ? (
                    <img
                      src={preview.url}
                      alt={file.name}
                      className="w-12 h-12 object-cover rounded"
                    />
                  ) : (
                    <svg
                      className="w-12 h-12 text-gray-400 dark:text-gray-300"
                      xmlns="http://www.w3.org/2000/svg"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                      aria-hidden="true"
                    >
                      <path
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth={2}
                        d="M12 4v16m8-8H4"
                      />
                    </svg>
                  )}
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-medium text-gray-900 dark:text-gray-100 truncate">
                      {file.name}
                    </p>
                    <p className="text-xs text-gray-500 dark:text-gray-400">
                      {(file.size / 1024).toFixed(1)} KB
                    </p>
                  </div>
                  <button
                    type="button"
                    onClick={() => removeFile(file)}
                    className="p-1 text-gray-500 hover:text-gray-700 dark:hover:text-gray-300 focus:outline-none focus:ring-2 focus:ring-primary-500 rounded"
                    aria-label={`Remove ${file.name}`}
                  >
                    <svg
                      className="w-4 h-4"
                      xmlns="http://www.w3.org/2000/svg"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                      aria-hidden="true"
                    >
                      <path
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth={2}
                        d="M6 18L18 6M6 6l12 12"
                      />
                    </svg>
                  </button>
                </li>
              );
            })}
          </ul>
        </aside>
      )}
    </section>
  );
};

export default UploadDropzone;