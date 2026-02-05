/**
 * Lightweight ANSI escape code parser.
 * Handles common terminal colors used in build logs.
 */

const ESC = "\x1b";
const ANSI_REGEX = new RegExp(`${ESC}\\[([0-9;]*)m`, "g");

// Standard colors (30-37) and bright colors (90-97)
const FOREGROUND_COLORS: Record<number, string> = {
  30: "#000000", // black
  31: "#cc0000", // red
  32: "#4e9a06", // green
  33: "#c4a000", // yellow
  34: "#3465a4", // blue
  35: "#75507b", // magenta
  36: "#06989a", // cyan
  37: "#d3d7cf", // white
  90: "#555753", // bright black (gray)
  91: "#ef2929", // bright red
  92: "#8ae234", // bright green
  93: "#fce94f", // bright yellow
  94: "#729fcf", // bright blue
  95: "#ad7fa8", // bright magenta
  96: "#34e2e2", // bright cyan
  97: "#eeeeec", // bright white
};

export interface AnsiSegment {
  text: string;
  color: string | null;
  bold: boolean;
}

/**
 * Parse text with ANSI escape codes into an array of styled segments.
 * @param text - Text containing ANSI escape codes
 * @returns Array of segments with text and style info
 */
export function parseAnsi(text: string): AnsiSegment[] {
  const segments: AnsiSegment[] = [];
  let lastIndex = 0;
  let currentColor: string | null = null;
  let isBold = false;

  for (const match of text.matchAll(ANSI_REGEX)) {
    // Add text before this escape code
    const textBefore = text.slice(lastIndex, match.index);
    if (textBefore) {
      segments.push({ text: textBefore, color: currentColor, bold: isBold });
    }

    const codes = match[1] ? match[1].split(";").map(Number) : [0];

    for (const code of codes) {
      if (code === 0) {
        // Reset all
        currentColor = null;
        isBold = false;
      } else if (code === 1) {
        // Bold
        isBold = true;
      } else if (FOREGROUND_COLORS[code]) {
        // Foreground color
        currentColor = FOREGROUND_COLORS[code];
      }
    }

    lastIndex = match.index! + match[0].length;
  }

  // Add remaining text
  const remaining = text.slice(lastIndex);
  if (remaining) {
    segments.push({ text: remaining, color: currentColor, bold: isBold });
  }

  return segments;
}

/**
 * Strip ANSI escape codes from text.
 * @param text - Text containing ANSI escape codes
 * @returns Plain text without escape codes
 */
export function stripAnsi(text: string): string {
  return text.replace(ANSI_REGEX, "");
}
