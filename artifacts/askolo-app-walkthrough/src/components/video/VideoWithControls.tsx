import { useCallback, useEffect, useRef, useState } from 'react';
import { ChevronDown, ChevronUp, Pause, Play, Repeat2, Volume2, VolumeX } from 'lucide-react';
import VideoTemplate, { SCENE_DURATIONS } from './VideoTemplate';
import { useSceneControls } from './useSceneControls';

const names: Record<string, string> = { overview: 'Overview', dashboard: 'Dashboard', planner: 'Planner', actions: 'Actions', close: 'Closing' };
const sceneFiles: Record<string, string> = {
  overview: 'src/components/video/video_scenes/Overview.tsx',
  dashboard: 'src/components/video/video_scenes/Dashboard.tsx',
  planner: 'src/components/video/video_scenes/Planner.tsx',
  actions: 'src/components/video/video_scenes/Actions.tsx',
  close: 'src/components/video/video_scenes/Close.tsx',
};
function mmss(ms: number) { return `${Math.floor(ms / 60000)}:${String(Math.floor(ms / 1000) % 60).padStart(2, '0')}`; }

export default function VideoWithControls() {
  const isIframed = typeof window !== 'undefined' && window.self !== window.top;
  const player = useSceneControls(SCENE_DURATIONS);
  const [muted, setMuted] = useState(false);
  const [collapsed, setCollapsed] = useState(false);
  const [hovered, setHovered] = useState(false);
  const [elapsed, setElapsed] = useState(0);
  const elapsedRef = useRef(0);
  const [visible, setVisible] = useState(true);

  useEffect(() => {
    elapsedRef.current = 0;
    setElapsed(0);
  }, [player.tick]);

  useEffect(() => {
    if (player.paused) return;
    const start = performance.now() - elapsedRef.current;
    const interval = window.setInterval(() => {
      elapsedRef.current = Math.min(performance.now() - start, player.activeDuration);
      setElapsed(elapsedRef.current);
    }, 60);
    return () => window.clearInterval(interval);
  }, [player.tick, player.paused, player.activeDuration]);

  useEffect(() => {
    if (!player.paused) return;
    const frozen = document.getAnimations().filter(animation => animation.playState === 'running');
    frozen.forEach(animation => animation.pause());
    return () => frozen.forEach(animation => animation.play());
  }, [player.paused]);

  const jumpTo = useCallback((index: number) => {
    player.jumpTo(index);
    const key = player.sceneKeys[index];
    window.parent.postMessage({
      type: 'REPLIT_VIDEO_SCENE_SELECTED',
      payload: { sceneIndex: index, sceneCount: player.sceneKeys.length, sceneTitle: names[key], filePath: sceneFiles[key], lineNumber: 1 },
    }, '*');
  }, [player]);

  if (!isIframed) return <VideoTemplate />;

  return (
    <div className="relative w-full h-screen overflow-hidden">
      <VideoTemplate key={player.mountKey} durations={player.durations} paused={player.paused}
        muted={muted} onSceneChange={player.onSceneChange} />
      <div className="absolute z-50 bottom-0 inset-x-0 h-[25%] flex items-end"
        onMouseEnter={() => setHovered(true)} onMouseLeave={() => setHovered(false)}
        onTouchStart={() => { if (collapsed) setCollapsed(false); }}>
        <div className={`w-full flex items-center gap-3 bg-[#0C141D]/95 border-t border-[#41546A] px-5 py-3 transition-all duration-200 text-[#E8F2F0] ${visible || hovered || !collapsed ? 'translate-y-0 opacity-100' : 'translate-y-full opacity-0 pointer-events-none'}`}>
          <button onClick={player.togglePause} aria-label={player.paused ? 'Play' : 'Pause'} className="p-3 rounded hover:bg-[#162332]">{player.paused ? <Play /> : <Pause />}</button>
          <button onClick={player.toggleLock} aria-label="Loop current scene" aria-pressed={player.locked} className={`p-3 rounded hover:bg-[#162332] ${player.locked ? 'text-[#9CC4F3]' : ''}`}><Repeat2 /></button>
          <button onClick={() => setMuted(current => !current)} aria-label={muted ? 'Unmute' : 'Mute'} className="p-3 rounded hover:bg-[#162332]">{muted ? <VolumeX /> : <Volume2 />}</button>
          <div className="h-8 w-px bg-[#41546A]" />
          <div className="flex flex-1 gap-1">
            {player.sceneKeys.map((key, index) => (
              <button key={key} aria-label={`Jump to ${names[key]}`} title={names[key]} onClick={() => jumpTo(index)}
                className="h-6 rounded bg-[#41546A] overflow-hidden relative"
                style={{ flex: (SCENE_DURATIONS as Record<string, number>)[key] }}>
                <span className="absolute left-0 top-0 bottom-0 bg-[#9CC4F3]"
                  style={{ width: index < player.activeIndex ? '100%' : index > player.activeIndex ? '0%' : `${Math.min(100, elapsed / player.activeDuration * 100)}%` }} />
              </button>
            ))}
          </div>
          <span className="text-sm whitespace-nowrap">{player.activeIndex + 1}/{player.sceneKeys.length}</span>
          <span className="text-sm whitespace-nowrap text-[#B2C4C1]">{mmss(player.activeStartTime + elapsed)} / {mmss(player.totalDuration)}</span>
          <button aria-label={collapsed ? 'Show controls' : 'Hide controls'} className="p-3 rounded hover:bg-[#162332]"
            onClick={() => { setCollapsed(!collapsed); setVisible(collapsed); setHovered(false); }}>
            {collapsed ? <ChevronUp /> : <ChevronDown />}
          </button>
        </div>
      </div>
    </div>
  );
}