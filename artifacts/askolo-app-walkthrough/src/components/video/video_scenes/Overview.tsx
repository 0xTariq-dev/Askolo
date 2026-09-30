import { motion } from 'framer-motion';
import { useSceneTimer } from '@/lib/video';
import { useState } from 'react';
import { ImageFrame, Mark, Shot } from './Visual';

export function Overview() {
  const [beat, setBeat] = useState(0);
  useSceneTimer([
    { time: 1100, callback: () => setBeat(1) },
    { time: 3300, callback: () => setBeat(2) },
  ]);

  return (
    <Shot>
      <div style={{ position: 'absolute', left: '6%', top: '8%', right: '6%', display: 'flex', alignItems: 'center', gap: '2.3vmin' }}>
        <Mark className="w-[8vmin] h-[8vmin] object-contain" />
        <div>
          <p className="micro m-0">Your day, together</p>
          <h1 className="headline" style={{ fontSize: '7.6vmin' }}>Askolo</h1>
        </div>
        <p className="subline ml-auto" style={{ fontSize: '2.5vmin' }}>A calm workspace for the moving pieces</p>
      </div>
      <motion.div
        style={{ position: 'absolute', left: '6%', right: '6%', top: '25%', height: '67%' }}
        initial={{ scale: 1.14, y: '5%' }}
        animate={{ scale: 1, y: 0 }}
        transition={{ duration: 4.7, ease: [0.16, 1, 0.3, 1] }}
      >
        <ImageFrame image="dashboard" className="w-full h-full" />
      </motion.div>
      <motion.div className="rule absolute left-[7%] bottom-[4.6%]" initial={{ width: 0 }} animate={{ width: beat >= 1 ? '17vmin' : 0 }} transition={{ duration: .6 }} />
      <motion.p className="annotation absolute right-[8%] bottom-[3.7%] m-0" initial={{ opacity: 0, y: 16 }} animate={{ opacity: beat >= 2 ? 1 : 0, y: beat >= 2 ? 0 : 16 }}>
        Dashboard · Planner · Actions
      </motion.p>
    </Shot>
  );
}