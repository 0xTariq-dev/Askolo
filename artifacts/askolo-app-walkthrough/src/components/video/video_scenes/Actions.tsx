import { motion } from 'framer-motion';
import { useState } from 'react';
import { useSceneTimer } from '@/lib/video';
import { ImageFrame, Shot } from './Visual';

const stages = ['Transcribe', 'Propose', 'Confirm'];

export function Actions() {
  const [beat, setBeat] = useState(0);
  useSceneTimer([
    { time: 1800, callback: () => setBeat(1) },
    { time: 3600, callback: () => setBeat(2) },
    { time: 5400, callback: () => setBeat(3) },
    { time: 7600, callback: () => setBeat(4) },
  ]);
  return (
    <Shot style="reveal">
      <p className="micro absolute left-[6%] top-[7%] m-0">Actions</p>
      <h2 className="headline absolute left-[6%] top-[11%]" style={{ fontSize: '7vmin' }}>One clear action</h2>
      <motion.div className="absolute left-[6%] top-[29%] w-[61%] h-[56%]" initial={{ scale: .91 }} animate={{ scale: beat >= 4 ? 1 : .96 }} transition={{ duration: .85 }}>
        <ImageFrame image="actions" className="w-full h-full" />
      </motion.div>
      <div className="absolute right-[5%] top-[30%] w-[25%] flex flex-col gap-[3vmin]">
        {stages.map((stage, i) => (
          <motion.div key={stage} className="flex items-center gap-[1.6vmin] border-b border-[#41546A] pb-[2vmin]"
            initial={{ opacity: .28, x: 10 }}
            animate={{ opacity: beat >= i + 1 ? 1 : .28, x: beat >= i + 1 ? 0 : 10 }}>
            <span className="text-[2vmin] text-[#9CC4F3]">0{i + 1}</span>
            <span className="annotation">{stage}</span>
          </motion.div>
        ))}
        <motion.p className="annotation" initial={{ opacity: 0, y: 10 }} animate={{ opacity: beat >= 3 ? 1 : 0, y: beat >= 3 ? 0 : 10 }}>
          Nothing saved until you confirm.
        </motion.p>
      </div>
      <p className="quiet absolute left-[6%] bottom-[5%] m-0">Illustrative sequence · interface captures do not show a completed result</p>
    </Shot>
  );
}