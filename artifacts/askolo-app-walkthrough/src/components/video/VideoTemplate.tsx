import {
  VideoCanvas,
  VideoPausedContext,
  type VideoAspectRatio,
  useVideoPlayer,
} from '@/lib/video';
import { AnimatePresence } from 'framer-motion';
import { useEffect, useRef, type ComponentType } from 'react';
import { Overview } from './video_scenes/Overview';
import { Dashboard } from './video_scenes/Dashboard';
import { Planner } from './video_scenes/Planner';
import { Actions } from './video_scenes/Actions';
import { Close } from './video_scenes/Close';

export const SCENE_DURATIONS = {
  overview: 6000,
  dashboard: 8000,
  planner: 12000,
  actions: 10000,
  close: 8000,
};

const VIDEO_ASPECT_RATIO: VideoAspectRatio = '16:9';
const SCENE_COMPONENTS: Record<string, ComponentType> = { overview: Overview, dashboard: Dashboard, planner: Planner, actions: Actions, close: Close };
const SCENE_START_SEC = Object.entries(SCENE_DURATIONS).reduce<Record<string, number>>((offsets, [key, ms]) => {
  offsets[key] = Object.entries(offsets).length ? Object.values(SCENE_DURATIONS).slice(0, Object.keys(offsets).length).reduce((sum, duration) => sum + duration, 0) / 1000 : 0;
  void ms;
  return offsets;
}, {});

export default function VideoTemplate({
  durations = SCENE_DURATIONS,
  loop = true,
  paused = false,
  muted = false,
  onSceneChange,
}: {
  durations?: Record<string, number>;
  loop?: boolean;
  paused?: boolean;
  muted?: boolean;
  onSceneChange?: (sceneKey: string) => void;
} = {}) {
  const { currentSceneKey } = useVideoPlayer({ durations, loop, paused });
  useEffect(() => { onSceneChange?.(currentSceneKey); }, [currentSceneKey, onSceneChange]);
  const baseSceneKey = currentSceneKey.replace(/_r[12]$/, '');
  const Scene = SCENE_COMPONENTS[baseSceneKey];
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const lastSceneKeyRef = useRef<string | null>(null);
  useEffect(() => {
    const audio = audioRef.current;
    if (!audio) return;
    if (paused) { audio.pause(); return; }
    if (lastSceneKeyRef.current !== currentSceneKey) {
      lastSceneKeyRef.current = currentSceneKey;
      const target = SCENE_START_SEC[baseSceneKey] ?? 0;
      if (Math.abs(audio.currentTime - target) > 0.18) audio.currentTime = target;
    }
    audio.play().catch(() => {});
  }, [currentSceneKey, baseSceneKey, paused, muted]);

  return (
    <>
      <VideoCanvas aspectRatio={VIDEO_ASPECT_RATIO} style={{ backgroundColor: 'var(--color-bg-light)' }}>
        <VideoPausedContext.Provider value={paused}>
          <AnimatePresence mode="sync">
            {Scene && <Scene key={currentSceneKey} />}
          </AnimatePresence>
        </VideoPausedContext.Provider>
      </VideoCanvas>
      <audio ref={audioRef} src={`${import.meta.env.BASE_URL}audio/composite_audio.mp3`} preload="auto" autoPlay muted={muted} />
    </>
  );
}
