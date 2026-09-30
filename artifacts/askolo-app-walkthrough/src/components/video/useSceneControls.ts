import { useCallback, useMemo, useState } from 'react';

export function useSceneControls(baseDurations: Record<string, number>) {
  const sceneKeys = useMemo(() => Object.keys(baseDurations), [baseDurations]);
  const [activeIndex, setActiveIndex] = useState(0);
  const [locked, setLocked] = useState(false);
  const [paused, setPaused] = useState(false);
  const [mountKey, setMountKey] = useState(0);
  const [tick, setTick] = useState(0);
  const durations = useMemo(() => {
    if (locked) {
      const key = sceneKeys[activeIndex];
      return { [`${key}_r1`]: baseDurations[key], [`${key}_r2`]: baseDurations[key] };
    }
    const result: Record<string, number> = {};
    sceneKeys.forEach((_, index) => {
      const key = sceneKeys[(index + activeIndex) % sceneKeys.length];
      result[key] = baseDurations[key];
    });
    return result;
  }, [locked, activeIndex, sceneKeys, baseDurations]);
  const totalDuration = Object.values(baseDurations).reduce((total, ms) => total + ms, 0);
  const activeStartTime = sceneKeys.slice(0, activeIndex).reduce((total, key) => total + baseDurations[key], 0);
  const onSceneChange = useCallback((rawKey: string) => {
    const index = sceneKeys.indexOf(rawKey.replace(/_r[12]$/, ''));
    if (index >= 0) setActiveIndex(index);
    setTick(t => t + 1);
  }, [sceneKeys]);
  const jumpTo = useCallback((index: number) => {
    setActiveIndex(index); setPaused(false); setMountKey(k => k + 1); setTick(t => t + 1);
  }, []);
  const toggleLock = useCallback(() => {
    setLocked(current => !current); setPaused(false); setMountKey(k => k + 1); setTick(t => t + 1);
  }, []);
  return {
    sceneKeys, activeIndex, locked, paused, mountKey, tick, durations,
    activeDuration: baseDurations[sceneKeys[activeIndex]] ?? 0,
    activeStartTime, totalDuration, onSceneChange, jumpTo, toggleLock,
    togglePause: () => setPaused(current => !current),
  };
}