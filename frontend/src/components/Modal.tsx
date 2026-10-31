import React, {
  useEffect,
  useRef,
  useState,
  ReactNode,
  KeyboardEvent,
  MouseEvent,
} from "react";
import { createPortal } from "react-dom";

type ModalSize = "sm" | "md" | "lg" | "xl";

interface ModalProps {
  /** Controls visibility of the modal */
  isOpen: boolean;
  /** Callback invoked when the modal should be closed */
  onClose: () => void;
  /** Optional title displayed in the header */
  title?: string;
  /** Modal body */
  children: ReactNode;
  /** Size of the modal content */
  size?: ModalSize;
  /** Additional class names for the content container */
  className?: string;
}

/**
 * Returns a list of focusable elements inside a container.
 */
function getFocusableElements(container: HTMLElement): HTMLElement[] {
  const focusableSelectors = [
    "a[href]",
    "area[href]",
    "input:not([disabled]):not([type='hidden'])",
    "select:not([disabled])",
    "textarea:not([disabled])",
    "button:not([disabled])",
    "iframe",
    "object",
    "embed",
    "[tabindex]:not([tabindex='-1'])",
    "[contenteditable]",
  ];
  const nodes = container.querySelectorAll<HTMLElement>(
    focusableSelectors.join(",")
  );
  return Array.from(nodes).filter((el) => el.offsetWidth > 0 || el.offsetHeight > 0);
}

/**
 * Generic, accessible modal component.
 *
 * - Renders via a portal to `document.body`.
 * - Traps focus while open.
 * - Closes on Escape, overlay click, or explicit `onClose`.
 * - Provides ARIA attributes and a smooth fade/scale animation.
 * - Disables body scroll while open.
 */
export const Modal: React.FC<ModalProps> = ({
  isOpen,
  onClose,
  title,
  children,
  size = "md",
  className = "",
}) => {
  const overlayRef = useRef<HTMLDivElement>(null);
  const dialogRef = useRef<HTMLDivElement>(null);
  const previouslyFocused = useRef<Element | null>(null);
  const [show, setShow] = useState(isOpen);

  // Map size to Tailwind max‑width utilities
  const sizeClasses: Record<ModalSize, string> = {
    sm: "max-w-sm",
    md: "max-w-md",
    lg: "max-w-lg",
    xl: "max-w-xl",
  };

  // Open / close animation handling
  useEffect(() => {
    if (isOpen) {
      setShow(true);
    } else {
      // Wait for animation to finish before unmounting
      const timeout = setTimeout(() => setShow(false), 200);
      return () => clearTimeout(timeout);
    }
  }, [isOpen]);

  // Body scroll lock
  useEffect(() => {
    if (isOpen) {
      document.body.style.overflow = "hidden";
    } else {
      document.body.style.overflow = "";
    }
    return () => {
      document.body.style.overflow = "";
    };
  }, [isOpen]);

  // Focus management
  useEffect(() => {
    if (!isOpen) return;

    previouslyFocused.current = document.activeElement;
    const focusable = dialogRef.current
      ? getFocusableElements(dialogRef.current)
      : [];

    // Focus the first focusable element or the dialog itself
    if (focusable.length) {
      focusable[0].focus();
    } else if (dialogRef.current) {
      dialogRef.current.focus();
    }

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.stopPropagation();
        onClose();
        return;
      }

      if (e.key === "Tab") {
        const focusables = dialogRef.current
          ? getFocusableElements(dialogRef.current)
          : [];
        if (focusables.length === 0) {
          e.preventDefault();
          return;
        }

        const first = focusables[0];
        const last = focusables[focusables.length - 1];
        if (e.shiftKey) {
          if (document.activeElement === first) {
            e.preventDefault();
            last.focus();
          }
        } else {
          if (document.activeElement === last) {
            e.preventDefault();
            first.focus();
          }
        }
      }
    };

    document.addEventListener("keydown", handleKeyDown as any);
    return () => {
      document.removeEventListener("keydown", handleKeyDown as any);
      // Restore focus to the element that opened the modal
      if (previouslyFocused.current instanceof HTMLElement) {
        previouslyFocused.current.focus();
      }
    };
  }, [isOpen, onClose]);

  // Click on overlay to close
  const handleOverlayClick = (e: MouseEvent<HTMLDivElement>) => {
    if (e.target === overlayRef.current) {
      onClose();
    }
  };

  if (!show) return null;

  const titleId = title ? "modal-title" : undefined;

  return createPortal(
    <div
      ref={overlayRef}
      className={`
        fixed inset-0 z-50 flex items-center justify-center
        bg-black bg-opacity-50 backdrop-blur-sm
        transition-opacity duration-200
        ${isOpen ? "opacity-100" : "opacity-0"}
      `}
      aria-labelledby={titleId}
      aria-modal="true"
      role="dialog"
      onMouseDown={handleOverlayClick}
    >
      <div
        ref={dialogRef}
        className={`
          bg-white dark:bg-gray-800 rounded-lg shadow-xl
          transform transition-all duration-200
          ${isOpen ? "opacity-100 translate-y-0 scale-100" : "opacity-0 translate-y-4 scale-95"}
          ${sizeClasses[size]} w-full mx-4
          focus:outline-none
          ${className}
        `}
        tabIndex={-1}
      >
        <div className="flex justify-between items-center p-4 border-b border-gray-200 dark:border-gray-700">
          {title && (
            <h2
              id={titleId}
              className="text-lg font-medium text-gray-900 dark:text-gray-100"
            >
              {title}
            </h2>
          )}
          <button
            type="button"
            onClick={onClose}
            className={`
              text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200
              focus:outline-none focus:ring-2 focus:ring-primary-500 rounded
            `}
            aria-label="Close modal"
          >
            <svg
              className="h-5 w-5"
              xmlns="http://www.w3.org/2000/svg"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              aria-hidden="true"
            >
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
        <div className="p-4 overflow-y-auto max-h-[80vh]">{children}</div>
      </div>
    </div>,
    document.body
  );
};

export default Modal;