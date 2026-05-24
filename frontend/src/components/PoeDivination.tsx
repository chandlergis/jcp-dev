import React, { useState, useCallback, useRef, useMemo, Suspense } from 'react';
import { Sparkles, RotateCw } from 'lucide-react';
import { Canvas, useFrame } from '@react-three/fiber';
import { ContactShadows, Environment, OrbitControls } from '@react-three/drei';
import * as THREE from 'three';

// ============== 类型与常量 ==============

export type PoeResult = 'sheng' | 'xiao' | 'yin';

export interface PoeRecord {
  result: PoeResult;
  timestamp: number;
}

interface PoeDivinationProps {
  question: string;
}

const RESULT_LABELS: Record<PoeResult, { name: string; emoji: string; color: string; desc: string }> = {
  sheng: {
    name: '圣杯',
    emoji: '🙏',
    color: 'text-amber-300',
    desc: '妈祖应允 · 一正一反',
  },
  xiao: {
    name: '笑杯',
    emoji: '😏',
    color: 'text-sky-300',
    desc: '妈祖含笑 · 双正向天',
  },
  yin: {
    name: '阴杯',
    emoji: '⚠️',
    color: 'text-rose-400',
    desc: '妈祖未允 · 双反伏地',
  },
};

// ============== 圣杯几何体 ==============
// 圣杯本体是个"半月形"木块：底部是平面（弧形边缘）、上半部分是凸面（半个橄榄球）
// 用 LatheGeometry 旋转一条曲线生成。

function buildPoeGeometry() {
  // 定义截面曲线点：从底面中心 → 边缘 → 顶部凸面 → 顶点
  const pts: THREE.Vector2[] = [];
  // 底面（平的）
  pts.push(new THREE.Vector2(0.0001, 0));
  pts.push(new THREE.Vector2(0.5, 0));
  // 外侧倒角
  pts.push(new THREE.Vector2(0.58, 0.08));
  // 顶部凸面 (半椭圆)
  const topSteps = 18;
  for (let i = 1; i <= topSteps; i++) {
    const t = i / topSteps;
    const r = 0.58 * Math.cos(t * Math.PI * 0.5);
    const y = 0.08 + 0.42 * Math.sin(t * Math.PI * 0.5);
    pts.push(new THREE.Vector2(Math.max(0.0001, r), y));
  }
  const geom = new THREE.LatheGeometry(pts, 48);
  geom.computeVertexNormals();
  // 缩放成更扁的半月形
  geom.scale(1.0, 0.55, 1.6);
  return geom;
}

// ============== 木质材质 ==============

function buildWoodTexture(): THREE.CanvasTexture {
  const size = 256;
  const cv = document.createElement('canvas');
  cv.width = size;
  cv.height = size;
  const ctx = cv.getContext('2d')!;
  // 基底
  const grad = ctx.createLinearGradient(0, 0, size, size);
  grad.addColorStop(0, '#a16207');
  grad.addColorStop(0.5, '#854d0e');
  grad.addColorStop(1, '#713f12');
  ctx.fillStyle = grad;
  ctx.fillRect(0, 0, size, size);
  // 木纹
  ctx.globalAlpha = 0.45;
  for (let i = 0; i < 28; i++) {
    const y = (i / 28) * size + (Math.random() - 0.5) * 6;
    ctx.strokeStyle = i % 3 === 0 ? '#3f2106' : '#5b3210';
    ctx.lineWidth = 0.5 + Math.random() * 1.8;
    ctx.beginPath();
    ctx.moveTo(0, y);
    let x = 0;
    while (x < size) {
      const wave = Math.sin(x * 0.04 + i) * 3 + (Math.random() - 0.5) * 2;
      ctx.lineTo(x, y + wave);
      x += 6;
    }
    ctx.stroke();
  }
  // 节疤
  ctx.globalAlpha = 0.5;
  for (let i = 0; i < 4; i++) {
    const cx = Math.random() * size;
    const cy = Math.random() * size;
    const rg = ctx.createRadialGradient(cx, cy, 1, cx, cy, 18);
    rg.addColorStop(0, '#2a1503');
    rg.addColorStop(1, 'rgba(42,21,3,0)');
    ctx.fillStyle = rg;
    ctx.beginPath();
    ctx.arc(cx, cy, 18, 0, Math.PI * 2);
    ctx.fill();
  }
  ctx.globalAlpha = 1;
  const tex = new THREE.CanvasTexture(cv);
  tex.wrapS = THREE.RepeatWrapping;
  tex.wrapT = THREE.RepeatWrapping;
  tex.colorSpace = THREE.SRGBColorSpace;
  return tex;
}

// ============== 物理掷杯：用关键帧近似 ==============
// 时间线 (秒)：
//   0     起飞，给随机线速度+角速度
//   0~0.9 在空中飞行（重力 9.8），旋转
//   0.9   触地，第一次弹跳（弹性 0.45）
//   1.4   第二次触地（弹性 0.25）
//   1.7   微弹一次后静止
//   1.8   ↓ 修正到目标朝向（"对齐"到正/反面）
//   2.2   完全静止

interface PoeBlockProps {
  position: [number, number, number]; // 起飞位置
  targetFaceUp: boolean; // 最终是否正面朝上
  throwKey: number; // 每次抛掷自增，触发新动画
  onLanded?: () => void;
}

const PoeBlock: React.FC<PoeBlockProps> = ({ position, targetFaceUp, throwKey }) => {
  const ref = useRef<THREE.Group>(null);
  const startedAt = useRef<number>(0);
  const geom = useMemo(() => buildPoeGeometry(), []);
  const woodTex = useMemo(() => buildWoodTexture(), []);

  // 随机化的旋转参数（每次掷都不同）
  const params = useMemo(() => {
    const seed = throwKey;
    // 用简单的伪随机
    const r = (n: number) => {
      const x = Math.sin(seed * 9.1 + n * 7.3) * 10000;
      return x - Math.floor(x);
    };
    return {
      spinX: 8 + r(1) * 6, // 总翻滚圈数
      spinY: 2 + r(2) * 4,
      spinZ: r(3) * 2,
      initVx: (r(4) - 0.5) * 1.2,
      initVz: (r(5) - 0.5) * 0.6,
      initVy: 4.5 + r(6) * 1.5,
      tiltDir: r(7) > 0.5 ? 1 : -1,
    };
  }, [throwKey]);

  useFrame(({ clock }) => {
    if (!ref.current) return;
    if (startedAt.current === 0) {
      startedAt.current = clock.elapsedTime;
    }
    const t = clock.elapsedTime - startedAt.current;
    const g = ref.current;

    // 物理参数
    const groundY = 0; // 圣杯底部高度（已经按几何体设计在 y=0 处）
    const startY = position[1]; // 起始离地高度
    const gravity = 18;

    if (t < 0.85) {
      // 起飞 + 自由落体
      const vy = params.initVy - gravity * t;
      const y = Math.max(groundY, startY + params.initVy * t - 0.5 * gravity * t * t);
      g.position.x = position[0] + params.initVx * t;
      g.position.z = position[2] + params.initVz * t;
      g.position.y = y;
      // 翻滚
      g.rotation.x = t * Math.PI * 2 * params.spinX * 0.4;
      g.rotation.y = t * Math.PI * 2 * params.spinY * 0.3;
      g.rotation.z = t * Math.PI * 2 * params.spinZ * 0.2;
      // 平移给一点点最终位置
      void vy;
    } else if (t < 1.8) {
      // 弹跳阶段：两次小弹跳
      const localT = t - 0.85;
      // 第一弹：0~0.35
      let y = 0;
      if (localT < 0.35) {
        const tt = localT / 0.35;
        y = Math.sin(tt * Math.PI) * 0.35;
      } else if (localT < 0.6) {
        const tt = (localT - 0.35) / 0.25;
        y = Math.sin(tt * Math.PI) * 0.12;
      } else {
        y = 0;
      }
      g.position.y = y;
      // 横向慢慢减速
      const decay = 1 - Math.min(1, localT / 0.95);
      g.position.x = position[0] + params.initVx * 0.85 + params.initVx * 0.15 * decay;
      g.position.z = position[2] + params.initVz * 0.85 + params.initVz * 0.15 * decay;
      // 旋转减速到稳定朝向
      const lerp = Math.min(1, localT / 0.95);
      const ease = lerp * lerp * (3 - 2 * lerp); // smoothstep
      const finalRotX = targetFaceUp ? 0 : Math.PI;
      const finalRotZ = params.tiltDir * 0.05; // 微微倾斜更真实
      const spinX = 0.85 * Math.PI * 2 * params.spinX * 0.4;
      g.rotation.x = spinX + (finalRotX - spinX) * ease;
      g.rotation.y = (1 - ease) * Math.PI * 2 * params.spinY * 0.3 * 0.85;
      g.rotation.z = (1 - ease) * Math.PI * 2 * params.spinZ * 0.2 * 0.85 + ease * finalRotZ;
    } else {
      // 静止
      g.position.y = 0;
      g.position.x = position[0] + params.initVx * 0.85 + params.initVx * 0.15;
      g.position.z = position[2] + params.initVz * 0.85 + params.initVz * 0.15;
      g.rotation.x = targetFaceUp ? 0 : Math.PI;
      g.rotation.z = params.tiltDir * 0.05;
    }
  });

  // 当 throwKey 变化时重置时间
  React.useEffect(() => {
    startedAt.current = 0;
  }, [throwKey]);

  return (
    <group ref={ref} position={position}>
      {/* 主体木块 */}
      <mesh geometry={geom} castShadow receiveShadow>
        <meshStandardMaterial
          map={woodTex}
          roughness={0.65}
          metalness={0.1}
          color="#c08552"
        />
      </mesh>
      {/* 正面"福"字（用一个圆形面贴在底部下方 - 即正面） */}
      <mesh position={[0, -0.001, 0]} rotation={[Math.PI / 2, 0, 0]}>
        <ringGeometry args={[0.0, 0.45, 32]} />
        <meshStandardMaterial
          color="#7c2d12"
          transparent
          opacity={0.55}
          roughness={0.8}
        />
      </mesh>
    </group>
  );
};

// ============== 场景 ==============

interface SceneProps {
  throwKey: number;
  block1Face: boolean;
  block2Face: boolean;
}

const Scene: React.FC<SceneProps> = ({ throwKey, block1Face, block2Face }) => {
  return (
    <>
      {/* 环境光 */}
      <ambientLight intensity={0.45} />
      {/* 主光源 */}
      <directionalLight
        position={[3, 6, 3]}
        intensity={1.4}
        castShadow
        shadow-mapSize-width={1024}
        shadow-mapSize-height={1024}
        shadow-camera-left={-5}
        shadow-camera-right={5}
        shadow-camera-top={5}
        shadow-camera-bottom={-5}
      />
      {/* 暖色辅助光 */}
      <pointLight position={[-2, 2, 2]} intensity={0.6} color="#f59e0b" />
      {/* 冷色背光 */}
      <pointLight position={[2, 2, -3]} intensity={0.3} color="#f43f5e" />

      {/* 地面（庙宇红毯感） */}
      <mesh rotation={[-Math.PI / 2, 0, 0]} position={[0, -0.05, 0]} receiveShadow>
        <planeGeometry args={[20, 20]} />
        <meshStandardMaterial
          color="#3a1f1a"
          roughness={0.9}
          metalness={0.05}
        />
      </mesh>

      {/* 接触阴影更柔和 */}
      <ContactShadows
        position={[0, 0.001, 0]}
        opacity={0.55}
        scale={8}
        blur={2.2}
        far={3}
      />

      {/* 两个圣杯 */}
      <PoeBlock
        position={[-1.4, 3.2, 0]}
        targetFaceUp={block1Face}
        throwKey={throwKey}
      />
      <PoeBlock
        position={[1.4, 3.4, -0.2]}
        targetFaceUp={block2Face}
        throwKey={throwKey + 0.5}
      />

      {/* 环境贴图（让木头反射更自然） */}
      <Environment preset="apartment" />
    </>
  );
};

// ============== 主组件 ==============

export const PoeDivination: React.FC<PoeDivinationProps> = ({ question }) => {
  const [rolling, setRolling] = useState(false);
  const [block1Face, setBlock1Face] = useState(true);
  const [block2Face, setBlock2Face] = useState(true);
  const [result, setResult] = useState<PoeResult | null>(null);
  const [history, setHistory] = useState<PoeRecord[]>([]);
  const [throwKey, setThrowKey] = useState(1);
  const audioRef = useRef<AudioContext | null>(null);

  const playWoodSound = useCallback(() => {
    try {
      if (!audioRef.current) {
        const AC = window.AudioContext || (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext;
        audioRef.current = new AC();
      }
      const ctx = audioRef.current;
      const playKnock = (when: number, freq: number, vol: number) => {
        const osc = ctx.createOscillator();
        const gain = ctx.createGain();
        osc.type = 'triangle';
        osc.frequency.setValueAtTime(freq, ctx.currentTime + when);
        osc.frequency.exponentialRampToValueAtTime(freq * 0.3, ctx.currentTime + when + 0.18);
        gain.gain.setValueAtTime(vol, ctx.currentTime + when);
        gain.gain.exponentialRampToValueAtTime(0.01, ctx.currentTime + when + 0.18);
        osc.connect(gain).connect(ctx.destination);
        osc.start(ctx.currentTime + when);
        osc.stop(ctx.currentTime + when + 0.22);
      };
      // 第一次落地
      playKnock(0.85, 220, 0.4);
      playKnock(0.86, 160, 0.3);
      // 第二次弹起
      playKnock(1.2, 180, 0.25);
      // 微弹
      playKnock(1.45, 150, 0.15);
    } catch {/* ignore */}
  }, []);

  const handleThrow = useCallback(() => {
    if (rolling) return;
    setRolling(true);
    setResult(null);

    const f1 = Math.random() < 0.5;
    const f2 = Math.random() < 0.5;
    setBlock1Face(f1);
    setBlock2Face(f2);

    setThrowKey(k => k + 1);
    playWoodSound();

    setTimeout(() => {
      let r: PoeResult;
      if (f1 && f2) r = 'xiao';
      else if (!f1 && !f2) r = 'yin';
      else r = 'sheng';
      setResult(r);
      setHistory(prev => [{ result: r, timestamp: Date.now() }, ...prev].slice(0, 6));
      setRolling(false);
    }, 2200);
  }, [rolling, playWoodSound]);

  const handleReset = useCallback(() => {
    setHistory([]);
    setResult(null);
  }, []);

  const consecutiveSheng = (() => {
    let c = 0;
    for (const r of history) {
      if (r.result === 'sheng') c++;
      else break;
    }
    return c;
  })();

  return (
    <div className="flex flex-col h-full p-5 overflow-auto fin-scrollbar">
      {/* 问题区 */}
      <div className="mb-4 px-4 py-3 rounded-lg border fin-divider bg-gradient-to-br from-amber-500/10 to-rose-500/10">
        <div className="flex items-center gap-2 mb-1">
          <Sparkles className="w-4 h-4 text-amber-300" />
          <span className="text-sm fin-text-secondary">向妈祖请示</span>
        </div>
        <div className="text-base fin-text-primary font-medium">
          {question || '请先在上方输入要请示的问题'}
        </div>
      </div>

      {/* 3D 圣杯舞台 */}
      <div
        className="relative flex-1 rounded-xl border fin-divider overflow-hidden"
        style={{
          background:
            'radial-gradient(ellipse at 50% 30%, rgba(251,191,36,0.18) 0%, rgba(0,0,0,0) 60%), linear-gradient(180deg, #2c1810 0%, #1a0f0a 100%)',
          minHeight: 320,
        }}
      >
        {/* 烛台装饰 */}
        <div className="absolute top-3 left-1/2 -translate-x-1/2 text-3xl opacity-40 pointer-events-none z-10">
          🪔
        </div>

        {/* R3F Canvas */}
        <Canvas
          shadows
          camera={{ position: [0, 3.5, 5.5], fov: 45 }}
          dpr={[1, 2]}
          style={{ width: '100%', height: '100%' }}
        >
          <Suspense fallback={null}>
            <Scene
              throwKey={throwKey}
              block1Face={block1Face}
              block2Face={block2Face}
            />
            <OrbitControls
              enableZoom={false}
              enablePan={false}
              minPolarAngle={Math.PI / 6}
              maxPolarAngle={Math.PI / 2.2}
              autoRotate={!rolling}
              autoRotateSpeed={0.6}
            />
          </Suspense>
        </Canvas>

        {/* 投掷按钮 */}
        <div className="absolute bottom-5 left-1/2 -translate-x-1/2 z-10">
          <button
            onClick={handleThrow}
            disabled={rolling}
            className={`px-6 py-2.5 rounded-full text-sm font-semibold transition-all ${
              rolling
                ? 'bg-amber-900/50 text-amber-200/50 cursor-not-allowed'
                : 'bg-gradient-to-r from-amber-500 to-rose-500 text-white hover:scale-105 active:scale-95 shadow-lg shadow-amber-500/40'
            }`}
          >
            {rolling ? '掷杯中...' : '🙏 虔诚掷杯'}
          </button>
        </div>

        {/* 结果浮层 */}
        {result && !rolling && (
          <div className="absolute top-3 left-3 px-3 py-2 rounded-lg bg-black/60 backdrop-blur-sm border fin-divider z-10">
            <div className={`text-lg font-bold ${RESULT_LABELS[result].color}`}>
              {RESULT_LABELS[result].emoji} {RESULT_LABELS[result].name}
            </div>
            <div className="text-xs fin-text-tertiary mt-0.5">
              {RESULT_LABELS[result].desc}
            </div>
          </div>
        )}

        {/* 操作提示 */}
        <div className="absolute top-3 right-3 text-[10px] fin-text-tertiary z-10">
          鼠标拖拽可旋转视角
        </div>
      </div>

      {/* 历史记录 */}
      <div className="mt-4 flex items-center justify-between gap-3">
        <div className="flex-1">
          <div className="text-xs fin-text-tertiary mb-1.5">
            最近6次（最新在左）
            {consecutiveSheng >= 3 && (
              <span className="ml-2 text-amber-300 font-bold animate-pulse">
                ⭐ 连续{consecutiveSheng}次圣杯，妈祖正式允诺！
              </span>
            )}
          </div>
          <div className="flex gap-1.5">
            {history.length === 0 ? (
              <div className="text-xs fin-text-tertiary italic">尚未掷杯</div>
            ) : (
              history.map((rec, i) => (
                <div
                  key={i}
                  className={`px-2 py-1 rounded text-xs ${RESULT_LABELS[rec.result].color} bg-slate-800/60 border fin-divider`}
                  title={new Date(rec.timestamp).toLocaleTimeString()}
                >
                  {RESULT_LABELS[rec.result].emoji}
                </div>
              ))
            )}
          </div>
        </div>
        {history.length > 0 && (
          <button
            onClick={handleReset}
            className="p-1.5 rounded fin-hover transition-colors"
            title="清空记录"
          >
            <RotateCw className="w-4 h-4 fin-text-secondary" />
          </button>
        )}
      </div>

      <div className="mt-3 text-[10px] fin-text-tertiary text-center italic">
        ※ 圣杯解卦仅供心境调节，股市有风险，投资需理性
      </div>
    </div>
  );
};
