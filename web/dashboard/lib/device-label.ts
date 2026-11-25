
const browsers: [RegExp, string][] = [
  [/\bEdg(?:e|A|iOS)?\//, "Edge"],
  [/\b(?:OPR|Opera)\//, "Opera"],
  [/\bSamsungBrowser\//, "Samsung Internet"],
  [/\b(?:Firefox|FxiOS)\//, "Firefox"],
  [/\b(?:HeadlessChrome|Chrome|CriOS|Chromium)\//, "Chrome"],
  [/\bVersion\/[\d.]+.*Safari\//, "Safari"],
  [/^curl\//, "curl"],
];

const systems: [RegExp, string][] = [
  [/\b(?:iPhone|iPad|iPod)\b/, "iOS"],
  [/\bAndroid\b/, "Android"],
  [/\bCrOS\b/, "ChromeOS"],
  [/\bWindows\b/, "Windows"],
  [/\bMac OS X\b|\bMacintosh\b/, "macOS"],
  [/\bLinux\b/, "Linux"],
];

export function deviceLabel(userAgent: string): string {
  const ua = userAgent.trim();
  const browser = browsers.find(([pattern]) => pattern.test(ua))?.[1];
  const system = systems.find(([pattern]) => pattern.test(ua))?.[1];
  if (browser && system) return `${browser} on ${system}`;
  if (browser) return browser;
  if (system) return `Browser on ${system}`;
  return "Unknown device";
}
