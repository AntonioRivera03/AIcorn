import { useEffect, useRef, useState } from "react";

type RequestKey = { message: string; key: string };

const read = <T>(key: string, parse: (raw: string) => T, fallback: T): T => {
  try {
    const raw = localStorage.getItem(key);
    return raw === null ? fallback : parse(raw);
  } catch {
    return fallback;
  }
};

const write = (key: string, value: string | null) => {
  try {
    if (value === null) localStorage.removeItem(key);
    else localStorage.setItem(key, value);
  } catch {
    // Without storage the draft still works for this visit.
  }
};

// A chat's unsent message, kept across visits, plus the key its send uses.
// The key survives a failed send, so retrying can't post the message twice.
export function useComposerDraft(storageKey: string) {
  const [message, setMessage] = useState(() => read(storageKey, (raw) => raw, ""));
  const requestKey = useRef<RequestKey | null>(
    read(
      `${storageKey}-request`,
      (raw) => {
        const value = JSON.parse(raw) as Partial<RequestKey>;
        return typeof value.message === "string" && typeof value.key === "string"
          ? (value as RequestKey)
          : null;
      },
      null,
    ),
  );

  useEffect(() => write(storageKey, message || null), [storageKey, message]);

  // The key for sending this message: the same one again on a retry.
  const keyFor = (text: string) => {
    if (requestKey.current?.message !== text)
      requestKey.current = { message: text, key: crypto.randomUUID() };
    write(`${storageKey}-request`, JSON.stringify(requestKey.current));
    return requestKey.current.key;
  };

  const sent = (text: string) => {
    requestKey.current = null;
    write(`${storageKey}-request`, null);
    setMessage((current) => (current === text ? "" : current));
  };

  return { message, setMessage, keyFor, sent };
}
