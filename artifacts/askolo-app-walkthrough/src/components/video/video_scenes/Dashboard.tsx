import { motion } from 'framer-motion';
import { useState } from 'react';
import { useSceneTimer } from '@/lib/video';
import { ImageFrame, Shot } from './Visual';

export function Dashboard() {
  const [beat, setBeat] = useState(0);
  useSceneTimer([
    { time: 2200, callback: () => setBeat(1) },
    { time: 4100, callback: () => setBeat(2) },
    { time: 6500, callback: () => setBeat(3) },
  ]);

  return (
    <Shot style="split">
      <motion.div
        className="absolute left-[5%] top-[13%] w-[66%] h-[74%]"
        initial={{ scale: 1.12, x: '-2%' }}
        animate={{ scale: beat >= 2 ? 1 : 1.06, x: beat >= 1 ? '0%' : '-2%' }}
        transition={{ duration: 1.4, ease: [0.2, 1, 0.3, 1] }}
      >
        <ImageFrame image="dashboard" className="w-full h-full" />
      </motion.div>
      <div className="absolute right-[5%] top-[23%] w-[22%]">
        <p className="micro">Dashboard</p>
        <h2 className="headline" style={{ fontSize: '6.6vmin' }}>A calmer view of today</h2>
        <motion.div className="rule mt-[4vmin]" initial={{ width: 0 }} animate={{ width: beat >= 1 ? '13vmin' : 0 }} />
        <motion.p className="annotation mt-[4vmin]" initial={{ opacity: 0, y: 16 }} animate={{ opacity: beat >= 1 ? 1 : 0, y: beat >= 1 ? 0 : 16 }}>
          {beat >= 2 ? 'Habits · Goals · Chores' : 'Plan'}
        </motion.p>
      </div>
      <motion.div className="rule absolute bottom-[7%] right-[5%]" initial={{ width: 0 }} animate={{ width: beat >= 3 ? '18%' : 0 }} transition={{ duration: .7 }} />
    </Shot>
  );
}