import { useEffect, useRef } from 'react';
import './hero-animation.css';

type TunnelTheme = 'light' | 'dark';

export interface AskoloTunnelConceptProps {
  theme: TunnelTheme;
  reducedMotion: boolean;
  paused?: boolean;
}

const TAU = Math.PI * 2;
const LAYERS = 88;
const DOTS_PER_RING = 128;
const DOT_RADIUS = 0.8375;

function smoothstep(edge0: number, edge1: number, value: number) {
  const t = Math.max(0, Math.min(1, (value - edge0) / (edge1 - edge0)));
  return t * t * (3 - 2 * t);
}

function tunnelPath(depth: number): [number, number] {
  const easing = smoothstep(1, 4, depth);
  return [
    (0.16 * Math.sin(TAU * depth * 0.5) +
      0.32 * Math.sin(TAU * depth * 0.2 + 0.3)) *
      easing,
    (0.24 * Math.cos(TAU * depth * 0.3) +
      0.16 * Math.cos(TAU * depth * 0.1)) *
      easing,
  ];
}

function themeColor(tokenName: '--background' | '--primary'): string {
  const channels = window
    .getComputedStyle(document.documentElement)
    .getPropertyValue(tokenName)
    .trim();
  if (!channels) {
    throw new Error(`Askolo hero animation requires the ${tokenName} theme token.`);
  }
  return `hsl(${channels})`;
}

export function AskoloTunnelConcept({
  theme,
  reducedMotion,
  paused = false,
}: AskoloTunnelConceptProps) {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    const context = canvas?.getContext('2d', { alpha: false });
    if (!canvas || !context) return;

    let width = 0;
    let height = 0;
    let pixelRatio = 1;
    let elapsed = 3.2;
    let lastTimestamp = 0;
    let frame = 0;
    let disposed = false;

    const background = themeColor('--background');
    const ink = themeColor('--primary');

    const resize = () => {
      const bounds = canvas.getBoundingClientRect();
      width = Math.max(1, bounds.width);
      height = Math.max(1, bounds.height);
      pixelRatio = Math.min(window.devicePixelRatio || 1, 1.5);
      const nextWidth = Math.round(width * pixelRatio);
      const nextHeight = Math.round(height * pixelRatio);
      if (canvas.width !== nextWidth || canvas.height !== nextHeight) {
        canvas.width = nextWidth;
        canvas.height = nextHeight;
      }
    };

    const draw = () => {
      context.setTransform(1, 0, 0, 1, 0, 0);
      context.globalAlpha = 1;
      context.fillStyle = background;
      context.fillRect(0, 0, canvas.width, canvas.height);
      context.setTransform(
        height * pixelRatio,
        0,
        0,
        -height * pixelRatio,
        (width * pixelRatio) / 2,
        (height * pixelRatio) / 2,
      );

      const cameraDepth = elapsed * 0.38;
      const cameraOffset = tunnelPath(cameraDepth);
      const cameraPhase = cameraDepth % (4 / LAYERS);
      const pointRadius = DOT_RADIUS / height;
      const visibleRadius = Math.hypot(width / height / 2, 0.5) + 0.25;

      context.fillStyle = ink;
      for (let layer = 1; layer <= LAYERS; layer += 1) {
        let depth = 1 - layer / LAYERS;
        depth -= cameraPhase;

        const path = tunnelPath(cameraDepth + depth);
        const offsetX = path[0] - cameraOffset[0];
        const offsetY = path[1] - cameraOffset[1];
        const denominator = depth * 0.8 + 0.4;
        const ringRadius = 0.145 / (denominator * denominator);
        if (ringRadius > visibleRadius + Math.hypot(offsetX, offsetY)) continue;

        context.globalAlpha = Math.max(
          0,
          Math.min(1, 0.19 + 0.59 * (1 - depth)),
        );
        context.beginPath();
        for (let dot = 0; dot < DOTS_PER_RING; dot += 1) {
          const angle = (dot / DOTS_PER_RING) * TAU;
          const x = ringRadius * Math.cos(angle) - offsetX;
          const y = ringRadius * Math.sin(angle) - offsetY;
          context.moveTo(x + pointRadius, y);
          context.arc(x, y, pointRadius, 0, TAU);
        }
        context.fill();
      }
      context.globalAlpha = 1;
    };

    const isMoving = () =>
      !paused && !reducedMotion && !document.hidden && !disposed;

    const animate = (timestamp: number) => {
      if (!isMoving()) return;
      if (lastTimestamp) {
        elapsed += Math.min((timestamp - lastTimestamp) / 1000, 0.05);
      }
      lastTimestamp = timestamp;
      draw();
      frame = window.requestAnimationFrame(animate);
    };

    const restart = () => {
      window.cancelAnimationFrame(frame);
      lastTimestamp = 0;
      resize();
      if (isMoving()) {
        frame = window.requestAnimationFrame(animate);
      } else {
        draw();
      }
    };

    const observer = new ResizeObserver(restart);
    observer.observe(canvas);
    document.addEventListener('visibilitychange', restart);
    window.addEventListener('resize', restart, { passive: true });
    restart();

    return () => {
      disposed = true;
      window.cancelAnimationFrame(frame);
      observer.disconnect();
      document.removeEventListener('visibilitychange', restart);
      window.removeEventListener('resize', restart);
    };
  }, [paused, reducedMotion, theme]);

  return (
    <canvas
      ref={canvasRef}
      className="askolo-tunnel-concept"
      aria-hidden="true"
    />
  );
}