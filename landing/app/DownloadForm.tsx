"use client";

import { useState, type FormEvent } from "react";
import { DOWNLOAD_URL } from "./site";

export function DownloadForm() {
  const [status, setStatus] = useState<"idle" | "saving" | "success" | "error">("idle");
  const [error, setError] = useState("");

  async function download(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (status === "saving") return;
    if (!event.currentTarget.reportValidity()) return;

    const email = new FormData(event.currentTarget).get("email");
    setStatus("saving");
    setError("");

    try {
      const response = await fetch("/api/signups", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email }),
        signal: AbortSignal.timeout(15_000),
      });
      const result = await response.json().catch(() => null);

      if (!response.ok || result?.ok !== true) {
        setStatus("error");
        setError(result?.error || "We couldn't save your email. Please try again.");
        return;
      }

      setStatus("success");
      window.location.assign(DOWNLOAD_URL);
    } catch {
      setStatus("error");
      setError("We couldn't save your email. Please try again.");
    }
  }

  return (
    <div className="download">
      <form
        className="download-form"
        action="/api/signups"
        method="post"
        onSubmit={download}
      >
        <label className="visually-hidden" htmlFor="email">
          Email address
        </label>
        <input
          id="email"
          name="email"
          type="email"
          autoComplete="email"
          placeholder="Your email"
          aria-describedby="download-status"
          maxLength={254}
          disabled={status === "saving"}
          required
        />
        <button type="submit" disabled={status === "saving"} aria-busy={status === "saving"}>
          {status === "saving" ? "Saving…" : "Download for Mac"}
        </button>
      </form>
      <div id="download-status" className="download-status" role="status">
        {status === "error" && <p>{error}</p>}
        {status === "success" && (
          <p>
            You’re on the list. Your download is starting. <a href={DOWNLOAD_URL}>Try again</a>
          </p>
        )}
      </div>
    </div>
  );
}
