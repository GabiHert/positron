import { act, render, screen } from "@testing-library/react";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";

const handlers = new Map<string, (...a: unknown[]) => void>();
vi.mock("./api", () => ({
  on: (event: string, cb: (...a: unknown[]) => void) => {
    handlers.set(event, cb);
    return () => handlers.delete(event);
  },
}));

let mounts = 0;
vi.mock("./App", () => ({
  App: () => {
    const [n] = useState(() => ++mounts);
    return <p>mount {n}</p>;
  },
}));

import { EVENT_RESUME, Root } from "./Root";

describe("Root", () => {
  it("redraws the app from fresh data when the window comes back", () => {
    render(<Root />);
    expect(screen.getByText("mount 1")).toBeTruthy();
    act(() => handlers.get(EVENT_RESUME)?.());
    expect(screen.getByText("mount 2")).toBeTruthy();
  });
});
