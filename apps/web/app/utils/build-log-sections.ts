/**
 * Parses Docker buildx log output into structured, collapsible sections
 * with individual steps rendered as header rows.
 *
 * Docker buildx output follows a pattern where each step is prefixed with
 * `#N` and tagged with a stage like `[base 1/4]` or `[internal]`.
 * This parser groups those steps into user-friendly sections, and within
 * each section, groups log lines into discrete steps with extracted commands,
 * durations, and prefix-stripped output lines.
 *
 * ORDER GUARANTEE: Sections appear in the order their first log line occurs
 * in the input array. Steps within a section appear in the order their first
 * log line occurs. Output lines within a step preserve their original order.
 * The input array is iterated exactly once per pass and never reordered.
 */

export interface BuildLogStep {
  /** The command/description shown as the step header */
  command: string;
  /** Step status */
  status: "completed" | "running";
  /** Duration from the DONE line, null if still running */
  duration: number | null;
  /** Output lines with prefixes stripped, DONE lines excluded */
  outputLines: Array<{ message: string; level: string }>;
}

export interface BuildLogSection {
  /** User-friendly section name */
  name: string;
  /** Whether the section is completed, currently running, or hasn't started */
  status: "completed" | "running" | "pending";
  /** Total duration in seconds (sum of step durations), null if not yet available */
  duration: number | null;
  /** Ordered steps within this section */
  steps: BuildLogStep[];
}

// Matches lines like: #5 [base 1/4] FROM docker.io/...
// Captures: step number, stage name, step index, step total, remainder
const STEP_HEADER_REGEX = /^#(\d+)\s+\[(\S+)\s+\d+\/\d+\]\s+(.*)/;

// Matches lines like: #5 [internal] load build definition from Dockerfile
// Captures: step number, remainder
const INTERNAL_STEP_REGEX = /^#(\d+)\s+\[internal\]\s+(.*)/;

// Matches lines like: #5 DONE 3.4s
const DONE_REGEX = /^#(\d+)\s+DONE\s+([\d.]+)s/;

// Matches lines like: #22 exporting to image
const EXPORTING_REGEX = /^#(\d+)\s+exporting to image/;

// Matches lines like: #23 [auth] zeitwork/...
// Captures: step number, remainder
const AUTH_REGEX = /^#(\d+)\s+\[auth\]\s+(.*)/;

// Matches any line starting with #N (to associate with a step)
const STEP_LINE_REGEX = /^#(\d+)\s/;

// Matches the #N prefix, optionally followed by a Docker timestamp (e.g. #7 0.290 ...)
// Captures: step number, optional timestamp, rest of line
const PREFIX_REGEX = /^#\d+\s+(?:(\d+\.\d+)\s+)?(.*)/;

// Matches lines like: #0 building with "default" instance using docker driver
const BUILDER_REGEX = /^#(\d+)\s+building with\b/;

/**
 * Strip the #N prefix (and optional timestamp) from a log message.
 * Returns the meaningful content only.
 */
function stripPrefix(message: string): string {
  const match = message.match(PREFIX_REGEX);
  if (!match || !match[2]) return message;
  return match[2];
}

/**
 * Parse an array of build log entries into structured sections with steps.
 *
 * @param logs - Raw build log entries from the API (in insertion order)
 * @param logs - Raw build log entries from the API (in insertion order)
 * @param options.isBuilding - Whether the build is still in progress. When false,
 *   all sections and steps are forced to "completed" status regardless of DONE lines.
 * @returns Array of sections in display order, each containing ordered steps
 */
export function parseBuildLogSections(
  logs: Array<{ message: string; level: string }>,
  options?: { isBuilding?: boolean },
): BuildLogSection[] {
  const isBuilding = options?.isBuilding ?? true;
  if (!logs || logs.length === 0) {
    return [];
  }

  // --- First pass: build step-to-section mapping and extract step commands ---

  // Maps step number -> section key
  const stepToSection = new Map<number, string>();
  // Maps step number -> command string (extracted from the header line)
  const stepCommands = new Map<number, string>();
  // Ordered list of section keys as they first appear
  const sectionOrder: string[] = [];
  const sectionSet = new Set<string>();

  function ensureSection(key: string) {
    if (!sectionSet.has(key)) {
      sectionSet.add(key);
      sectionOrder.push(key);
    }
  }

  for (const log of logs) {
    const msg = log.message;

    // Skip builder info line entirely (#0 building with "default" instance ...)
    // This is a one-off informational line with no matching DONE marker.
    if (BUILDER_REGEX.test(msg)) {
      continue;
    }

    // Internal steps
    const internalMatch = msg.match(INTERNAL_STEP_REGEX);
    if (internalMatch) {
      const stepNum = parseInt(internalMatch[1]!, 10);
      stepToSection.set(stepNum, "internal");
      if (!stepCommands.has(stepNum)) {
        stepCommands.set(stepNum, capitalizeFirst(internalMatch[2]!));
      }
      ensureSection("internal");
      continue;
    }

    // Stage steps like [base 1/4] FROM ...
    const stepMatch = msg.match(STEP_HEADER_REGEX);
    if (stepMatch) {
      const stepNum = parseInt(stepMatch[1]!, 10);
      const stageName = stepMatch[2]!;
      stepToSection.set(stepNum, stageName);
      if (!stepCommands.has(stepNum)) {
        stepCommands.set(stepNum, stepMatch[3]!);
      }
      ensureSection(stageName);
      continue;
    }

    // Exporting steps
    const exportMatch = msg.match(EXPORTING_REGEX);
    if (exportMatch) {
      const stepNum = parseInt(exportMatch[1]!, 10);
      stepToSection.set(stepNum, "_export");
      if (!stepCommands.has(stepNum)) {
        stepCommands.set(stepNum, "Export and push image");
      }
      ensureSection("_export");
      continue;
    }

    // Auth steps
    const authMatch = msg.match(AUTH_REGEX);
    if (authMatch) {
      const stepNum = parseInt(authMatch[1]!, 10);
      stepToSection.set(stepNum, "_export");
      if (!stepCommands.has(stepNum)) {
        stepCommands.set(stepNum, `Authenticate ${authMatch[2]!}`);
      }
      ensureSection("_export");
      continue;
    }
  }

  // --- Build section and step structures ---

  interface StepAccumulator {
    command: string;
    duration: number | null;
    done: boolean;
    outputLines: Array<{ message: string; level: string }>;
  }

  interface SectionAccumulator {
    name: string;
    stepOrder: number[]; // step numbers in order of first appearance
    stepSet: Set<number>;
    steps: Map<number, StepAccumulator>;
  }

  const sections = new Map<string, SectionAccumulator>();

  for (const key of sectionOrder) {
    const name =
      key === "_export"
        ? "Export"
        : capitalizeFirst(key);
    sections.set(key, {
      name,
      stepOrder: [],
      stepSet: new Set(),
      steps: new Map(),
    });
  }

  function ensureStep(section: SectionAccumulator, stepNum: number) {
    if (!section.stepSet.has(stepNum)) {
      section.stepSet.add(stepNum);
      section.stepOrder.push(stepNum);
      section.steps.set(stepNum, {
        command: stepCommands.get(stepNum) ?? `Step #${stepNum}`,
        duration: null,
        done: false,
        outputLines: [],
      });
    }
  }

  // --- Second pass: assign each log line to its section/step ---

  for (const log of logs) {
    const msg = log.message;

    // Skip builder info line entirely
    if (BUILDER_REGEX.test(msg)) {
      continue;
    }

    // Extract step number
    const stepLineMatch = msg.match(STEP_LINE_REGEX);
    if (!stepLineMatch) {
      // Orphan line -- attach to last step of last section
      if (sectionOrder.length > 0) {
        const lastKey = sectionOrder[sectionOrder.length - 1]!;
        const lastSection = sections.get(lastKey)!;
        if (lastSection.stepOrder.length > 0) {
          const lastStepNum =
            lastSection.stepOrder[lastSection.stepOrder.length - 1]!;
          lastSection.steps.get(lastStepNum)!.outputLines.push({
            message: msg,
            level: log.level,
          });
        }
      }
      continue;
    }

    const stepNum = parseInt(stepLineMatch[1]!, 10);
    const sectionKey = stepToSection.get(stepNum) ?? "unknown";
    const section = sections.get(sectionKey);

    if (!section) {
      // Unknown step -- attach to last section's last step
      if (sectionOrder.length > 0) {
        const lastKey = sectionOrder[sectionOrder.length - 1]!;
        const lastSection = sections.get(lastKey)!;
        if (lastSection.stepOrder.length > 0) {
          const lastStepNum =
            lastSection.stepOrder[lastSection.stepOrder.length - 1]!;
          lastSection.steps.get(lastStepNum)!.outputLines.push({
            message: stripPrefix(msg),
            level: log.level,
          });
        }
      }
      continue;
    }

    ensureStep(section, stepNum);
    const step = section.steps.get(stepNum)!;

    // Check if this is a DONE line
    const doneMatch = msg.match(DONE_REGEX);
    if (doneMatch) {
      step.done = true;
      step.duration = parseFloat(doneMatch[2]!);
      // Don't add DONE lines to output
      continue;
    }

    // Check if this is a header/command line (already captured as command, skip)
    if (
      INTERNAL_STEP_REGEX.test(msg) ||
      STEP_HEADER_REGEX.test(msg) ||
      EXPORTING_REGEX.test(msg) ||
      AUTH_REGEX.test(msg) ||
      BUILDER_REGEX.test(msg)
    ) {
      // Don't add header lines to output -- the command is shown in the step header
      continue;
    }

    // Regular output line -- strip prefix and add
    const stripped = stripPrefix(msg);
    if (stripped !== "") {
      step.outputLines.push({ message: stripped, level: log.level });
    }
  }

  // --- Build final result ---

  const result: BuildLogSection[] = [];

  for (const key of sectionOrder) {
    const section = sections.get(key)!;
    if (section.stepOrder.length === 0) continue;

    const steps: BuildLogStep[] = [];
    let totalDuration = 0;
    let hasDuration = false;
    let allDone = true;

    for (const stepNum of section.stepOrder) {
      const step = section.steps.get(stepNum)!;
      const stepDone = step.done || !isBuilding;
      steps.push({
        command: step.command,
        status: stepDone ? "completed" : "running",
        duration: step.duration,
        outputLines: step.outputLines,
      });
      if (step.duration !== null) {
        totalDuration += step.duration;
        hasDuration = true;
      }
      if (!stepDone) {
        allDone = false;
      }
    }

    const sectionDuration = hasDuration
      ? Math.round(totalDuration * 100) / 100
      : null;

    result.push({
      name: section.name,
      status: allDone ? "completed" : "running",
      duration: sectionDuration,
      steps,
    });
  }

  // Fallback: if we couldn't parse sections, return a single section with one step
  if (result.length === 0 && logs.length > 0) {
    const fallbackStatus = isBuilding ? "running" : "completed";
    return [
      {
        name: "Build Output",
        status: fallbackStatus,
        duration: null,
        steps: [
          {
            command: "Build",
            status: fallbackStatus,
            duration: null,
            outputLines: logs.map((l) => ({
              message: l.message,
              level: l.level,
            })),
          },
        ],
      },
    ];
  }

  return result;
}

function capitalizeFirst(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1);
}

/**
 * Format a duration in seconds to a human-readable string.
 * @param seconds - Duration in seconds
 * @returns Formatted string like "1.2s", "45.0s", "1m 23s"
 */
export function formatDuration(seconds: number): string {
  if (seconds < 60) {
    return `${seconds.toFixed(1)}s`;
  }
  const mins = Math.floor(seconds / 60);
  const secs = Math.round(seconds % 60);
  return `${mins}m ${secs}s`;
}
