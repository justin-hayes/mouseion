/** Measure WCAG text contrast from the colors actually rendered by the browser. */
export function renderedTextContrast(element: Element): number {
  const canvas = document.createElement('canvas');
  canvas.width = 1;
  canvas.height = 1;
  const context = canvas.getContext('2d', { willReadFrequently: true });
  if (!context) throw new Error('Unable to create a canvas for color measurement');

  const parseColor = (value: string, label: string): [number, number, number, number] => {
    // Keep this check inside the function: Playwright serializes this function
    // into the browser page and does not carry module imports with it.
    if (!CSS.supports('color', value)) throw new Error(`Unsupported ${label} color: ${value}`);
    context.clearRect(0, 0, 1, 1);
    context.fillStyle = value;
    context.fillRect(0, 0, 1, 1);
    const channels = Array.from(context.getImageData(0, 0, 1, 1).data);
    if (channels.length !== 4 || channels.some(channel => !Number.isFinite(channel))) {
      throw new Error(`Unable to resolve ${label} color: ${value}`);
    }
    return [channels[0], channels[1], channels[2], channels[3] / 255];
  };

  const over = (
    foreground: [number, number, number, number],
    background: [number, number, number, number],
  ): [number, number, number, number] => {
    const alpha = foreground[3] + background[3] * (1 - foreground[3]);
    if (alpha === 0) return [0, 0, 0, 0];
    return [0, 1, 2].map(index => (
      (foreground[index] * foreground[3] + background[index] * background[3] * (1 - foreground[3])) / alpha
    )).concat(alpha) as [number, number, number, number];
  };

  // The initial document canvas is white when neither html nor body paints it.
  // Walk the rendered ancestry so local surfaces and transparent wrappers are
  // resolved instead of assuming every component sits on the page surface.
  const ancestry: Element[] = [];
  for (let node: Element | null = element; node; node = node.parentElement) ancestry.push(node);
  let background: [number, number, number, number] = [255, 255, 255, 1];
  for (const node of ancestry.reverse()) {
    const style = getComputedStyle(node);
    if (style.backgroundImage !== 'none' && !/^none(?:\s*,\s*none)*$/.test(style.backgroundImage)) {
      throw new Error(`Unsupported background image while measuring contrast: ${style.backgroundImage}`);
    }
    background = over(parseColor(style.backgroundColor, 'background'), background);
  }

  const foreground = over(parseColor(getComputedStyle(element).color, 'foreground'), background);
  const luminance = (color: [number, number, number, number]): number => {
    const linear = color.slice(0, 3).map(channel => {
      const value = channel / 255;
      return value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4;
    });
    return 0.2126 * linear[0] + 0.7152 * linear[1] + 0.0722 * linear[2];
  };

  const foregroundLuminance = luminance(foreground);
  const backgroundLuminance = luminance(background);
  const ratio = (Math.max(foregroundLuminance, backgroundLuminance) + 0.05)
    / (Math.min(foregroundLuminance, backgroundLuminance) + 0.05);
  if (!Number.isFinite(ratio)) throw new Error('Unable to calculate a finite text contrast ratio');
  return ratio;
}
