import { useEffect, useState } from "react";

export interface ClockState {
  now: Date;
  /** "Good morning | afternoon | evening | night" */
  greeting: string;
  /** 24h "14:05" */
  time: string;
  /** seconds, for an optional ticking colon */
  seconds: string;
  /** "Sunday, 16 June" */
  dateLabel: string;
}

function pad(n: number): string {
  return n.toString().padStart(2, "0");
}

function greetingFor(hour: number): string {
  if (hour < 5) return "Good night";
  if (hour < 12) return "Good morning";
  if (hour < 18) return "Good afternoon";
  return "Good evening";
}

const WEEKDAYS = [
  "Sunday",
  "Monday",
  "Tuesday",
  "Wednesday",
  "Thursday",
  "Friday",
  "Saturday",
];
const MONTHS = [
  "January",
  "February",
  "March",
  "April",
  "May",
  "June",
  "July",
  "August",
  "September",
  "October",
  "November",
  "December",
];

function build(d: Date): ClockState {
  return {
    now: d,
    greeting: greetingFor(d.getHours()),
    time: `${pad(d.getHours())}:${pad(d.getMinutes())}`,
    seconds: pad(d.getSeconds()),
    dateLabel: `${WEEKDAYS[d.getDay()]}, ${d.getDate()} ${MONTHS[d.getMonth()]}`,
  };
}

/** Live clock that re-renders every second. */
export function useClock(): ClockState {
  const [state, setState] = useState<ClockState>(() => build(new Date()));

  useEffect(() => {
    const id = window.setInterval(() => setState(build(new Date())), 1000);
    return () => window.clearInterval(id);
  }, []);

  return state;
}
