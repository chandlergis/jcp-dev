/**
 * 生辰八字今日运势计算
 * 简化版：基于天干地支 + 五行生克 + 日干旺衰，输出炒股相关运势
 *
 * 免责声明：本模块仅供娱乐参考，股市有风险，入市需谨慎，玄学不能替代理性分析。
 */

// 天干
export const HEAVENLY_STEMS = ['甲', '乙', '丙', '丁', '戊', '己', '庚', '辛', '壬', '癸'];
// 地支
export const EARTHLY_BRANCHES = ['子', '丑', '寅', '卯', '辰', '巳', '午', '未', '申', '酉', '戌', '亥'];
// 天干五行
const STEM_ELEMENT: Record<string, string> = {
  甲: '木', 乙: '木', 丙: '火', 丁: '火', 戊: '土',
  己: '土', 庚: '金', 辛: '金', 壬: '水', 癸: '水',
};
// 地支五行
const BRANCH_ELEMENT: Record<string, string> = {
  子: '水', 丑: '土', 寅: '木', 卯: '木', 辰: '土', 巳: '火',
  午: '火', 未: '土', 申: '金', 酉: '金', 戌: '土', 亥: '水',
};
// 五行生克
const ELEMENT_GENERATES: Record<string, string> = {
  木: '火', 火: '土', 土: '金', 金: '水', 水: '木',
};
const ELEMENT_OVERCOMES: Record<string, string> = {
  木: '土', 土: '水', 水: '火', 火: '金', 金: '木',
};
// 五行对应方位
const ELEMENT_DIRECTION: Record<string, string> = {
  木: '东方', 火: '南方', 土: '中央', 金: '西方', 水: '北方',
};
// 五行对应颜色
const ELEMENT_COLOR: Record<string, string> = {
  木: '青绿', 火: '朱红', 土: '明黄', 金: '银白', 水: '玄黑',
};
// 五行对应数字（河图洛书）
const ELEMENT_NUMBERS: Record<string, number[]> = {
  木: [3, 8], 火: [2, 7], 土: [5, 0], 金: [4, 9], 水: [1, 6],
};

// 时辰对应地支（24小时制 → 地支）
function hourToBranch(hour: number): string {
  // 23:00-1:00 子, 1:00-3:00 丑, ...
  const idx = Math.floor(((hour + 1) % 24) / 2);
  return EARTHLY_BRANCHES[idx];
}

// 简化版：根据公历年份计算年柱（以立春为界过于复杂，这里简化用元旦）
function getYearPillar(year: number): { stem: string; branch: string } {
  // 1984 = 甲子年
  const offset = year - 1984;
  const stem = HEAVENLY_STEMS[((offset % 10) + 10) % 10];
  const branch = EARTHLY_BRANCHES[((offset % 12) + 12) % 12];
  return { stem, branch };
}

// 简化版月柱（实际需用节气，这里近似）
function getMonthPillar(_year: number, month: number, yearStem: string): { stem: string; branch: string } {
  // 月支：寅月=正月（农历），这里粗略用公历
  const monthBranches = ['寅', '卯', '辰', '巳', '午', '未', '申', '酉', '戌', '亥', '子', '丑'];
  const branch = monthBranches[(month - 1) % 12];
  // 月干起例：甲己之年丙作首 (五虎遁)
  const yearStemIdx = HEAVENLY_STEMS.indexOf(yearStem);
  const monthStemStart = [2, 4, 6, 8, 0, 2, 4, 6, 8, 0][yearStemIdx]; // 丙丁戊己庚辛甲乙丙丁
  const stem = HEAVENLY_STEMS[(monthStemStart + (month - 1)) % 10];
  return { stem, branch };
}

// 日柱（使用蔡勒公式的简化）
function getDayPillar(year: number, month: number, day: number): { stem: string; branch: string } {
  // 基准日：1900-01-01 = 甲戌日 (干支序0号=甲子)
  // 甲戌的干支序为 10
  const baseDate = new Date(1900, 0, 1).getTime();
  const targetDate = new Date(year, month - 1, day).getTime();
  const diffDays = Math.floor((targetDate - baseDate) / 86400000);
  const ganZhiIdx = ((10 + diffDays) % 60 + 60) % 60;
  const stem = HEAVENLY_STEMS[ganZhiIdx % 10];
  const branch = EARTHLY_BRANCHES[ganZhiIdx % 12];
  return { stem, branch };
}

// 时柱（五鼠遁：甲己还加甲）
function getHourPillar(hour: number, dayStem: string): { stem: string; branch: string } {
  const branch = hourToBranch(hour);
  const dayStemIdx = HEAVENLY_STEMS.indexOf(dayStem);
  // 时干起例：甲己日甲子时 (五鼠遁)
  const hourStemStart = [0, 2, 4, 6, 8, 0, 2, 4, 6, 8][dayStemIdx];
  const branchIdx = EARTHLY_BRANCHES.indexOf(branch);
  const stem = HEAVENLY_STEMS[(hourStemStart + branchIdx) % 10];
  return { stem, branch };
}

export interface BaziPillars {
  year: { stem: string; branch: string };
  month: { stem: string; branch: string };
  day: { stem: string; branch: string };
  hour: { stem: string; branch: string };
  dayMaster: string; // 日干（命主）
  dayMasterElement: string; // 日主五行
}

export function calculateBazi(birthYear: number, birthMonth: number, birthDay: number, birthHour: number): BaziPillars {
  const year = getYearPillar(birthYear);
  const month = getMonthPillar(birthYear, birthMonth, year.stem);
  const day = getDayPillar(birthYear, birthMonth, birthDay);
  const hour = getHourPillar(birthHour, day.stem);
  return {
    year, month, day, hour,
    dayMaster: day.stem,
    dayMasterElement: STEM_ELEMENT[day.stem],
  };
}

// 计算五行统计
function countElements(pillars: BaziPillars): Record<string, number> {
  const counts: Record<string, number> = { 木: 0, 火: 0, 土: 0, 金: 0, 水: 0 };
  const all = [pillars.year, pillars.month, pillars.day, pillars.hour];
  all.forEach(p => {
    counts[STEM_ELEMENT[p.stem]]++;
    counts[BRANCH_ELEMENT[p.branch]]++;
  });
  return counts;
}

export interface DailyFortune {
  pillars: BaziPillars;
  todayPillar: { stem: string; branch: string };
  elementCounts: Record<string, number>;
  // 综合运势
  wealthScore: number; // 财运指数 0-100
  wealthLevel: string; // 大吉/中吉/平/小凶/大凶
  // 关系分析
  relation: string; // 日主与今日天干的关系
  // 宜忌
  suitable: string[];
  avoid: string[];
  // 幸运元素
  luckyDirection: string;
  luckyColor: string;
  luckyNumbers: number[];
  // 用神（喜用之五行）
  usefulElement: string;
  // 韭菜盘专属
  marketAdvice: string;
  sectorHint: string;
  // 一句诗
  poem: string;
}

const RELATIONS = {
  same: '比劫', // 同五行 (兄弟)
  generates: '食伤', // 我生 (子女)
  generated: '正印', // 生我 (父母)
  overcomes: '财星', // 我克 (财)
  overcome: '官杀', // 克我 (压力)
};

function getRelation(dayElement: string, otherElement: string): string {
  if (dayElement === otherElement) return RELATIONS.same;
  if (ELEMENT_GENERATES[dayElement] === otherElement) return RELATIONS.generates;
  if (ELEMENT_GENERATES[otherElement] === dayElement) return RELATIONS.generated;
  if (ELEMENT_OVERCOMES[dayElement] === otherElement) return RELATIONS.overcomes;
  if (ELEMENT_OVERCOMES[otherElement] === dayElement) return RELATIONS.overcome;
  return '中性';
}

// 板块对应五行（炒股专用映射）
const SECTOR_MAP: Record<string, string> = {
  木: '林业、造纸、家具、医药、农林牧渔、新能源',
  火: '能源、石油、化工、电力、传媒、芯片',
  土: '房地产、建材、水泥、陶瓷、农业',
  金: '银行、保险、证券、有色金属、机械',
  水: '物流、海运、渔业、旅游、酒水',
};

export function calculateDailyFortune(
  pillars: BaziPillars,
  date: Date = new Date()
): DailyFortune {
  const todayPillar = getDayPillar(date.getFullYear(), date.getMonth() + 1, date.getDate());
  const todayElement = STEM_ELEMENT[todayPillar.stem];
  const todayBranchElement = BRANCH_ELEMENT[todayPillar.branch];
  const dayElement = pillars.dayMasterElement;

  // 关系判定
  const stemRelation = getRelation(dayElement, todayElement);
  const branchRelation = getRelation(dayElement, todayBranchElement);

  // 五行统计 + 用神判定（简化：缺最少的五行）
  const counts = countElements(pillars);
  const usefulElement = Object.entries(counts).sort((a, b) => a[1] - b[1])[0][0];

  // 财运评分逻辑
  let score = 50;
  const relationScores: Record<string, number> = {
    [RELATIONS.same]: 8,         // 比劫：朋友帮忙，小有助益
    [RELATIONS.generates]: 5,    // 食伤：消耗，但是表现机会
    [RELATIONS.generated]: 15,   // 正印：贵人扶持
    [RELATIONS.overcomes]: 25,   // 财星：财来到我，大吉
    [RELATIONS.overcome]: -15,   // 官杀：压力大，慎
  };
  score += relationScores[stemRelation] || 0;
  score += (relationScores[branchRelation] || 0) * 0.7;

  // 用神出现加分
  if (todayElement === usefulElement || todayBranchElement === usefulElement) {
    score += 12;
  }
  // 忌神出现减分（克日主的）
  const enemyElement = Object.keys(ELEMENT_OVERCOMES).find(e => ELEMENT_OVERCOMES[e] === dayElement);
  if (enemyElement && (todayElement === enemyElement || todayBranchElement === enemyElement)) {
    score -= 8;
  }

  // 范围限制
  score = Math.max(5, Math.min(98, Math.round(score)));

  let level = '平';
  if (score >= 85) level = '大吉';
  else if (score >= 70) level = '中吉';
  else if (score >= 55) level = '小吉';
  else if (score >= 40) level = '持平';
  else if (score >= 25) level = '小凶';
  else level = '大凶';

  // 宜忌
  const suitable: string[] = [];
  const avoid: string[] = [];

  if (stemRelation === RELATIONS.overcomes || branchRelation === RELATIONS.overcomes) {
    suitable.push('追涨', '加仓', '主动出击');
  }
  if (stemRelation === RELATIONS.generated) {
    suitable.push('听取建议', '关注研报', '跟随机构');
  }
  if (stemRelation === RELATIONS.same) {
    suitable.push('与同行交流', '加入投资群');
  }
  if (stemRelation === RELATIONS.overcome) {
    avoid.push('追高', '满仓', '杠杆');
  }
  if (stemRelation === RELATIONS.generates) {
    avoid.push('过度交易', '频繁换股');
  }
  if (score < 40) {
    avoid.push('短线博弈', '冲动下单');
  }
  if (score >= 70) {
    suitable.push('盯盘操作');
  }
  if (suitable.length === 0) suitable.push('谨慎观察', '减少操作');
  if (avoid.length === 0) avoid.push('情绪化交易');

  // 韭菜盘建议
  let marketAdvice = '';
  if (score >= 85) marketAdvice = '★★★★★ 财星临门，今日宜大胆出手，但仍需止损纪律';
  else if (score >= 70) marketAdvice = '★★★★ 运势走旺，可适当加大仓位，关注热门板块';
  else if (score >= 55) marketAdvice = '★★★ 运势平稳，按既定计划操作';
  else if (score >= 40) marketAdvice = '★★ 运势一般，宜守不宜攻，控制仓位';
  else if (score >= 25) marketAdvice = '★ 运势偏弱，建议轻仓观望';
  else marketAdvice = '☆ 运势低迷，今日休息，多看少动';

  // 板块提示（用神对应的板块）
  const sectorHint = SECTOR_MAP[usefulElement];

  // 诗句
  const poems = [
    '财来财去本无常，从容应对自安康。',
    '云开见月分外明，韭菜地里春风生。',
    '盘中风云莫测变，唯有耐心是真金。',
    '青山遮不住，毕竟东流去。',
    '大江东去浪淘尽，A股盘前已开盘。',
    '股海无涯回头是岸，落袋为安方得平安。',
  ];
  const poem = poems[Math.floor(Math.random() * poems.length)];

  return {
    pillars,
    todayPillar,
    elementCounts: counts,
    wealthScore: score,
    wealthLevel: level,
    relation: `日主${dayElement}遇${todayElement}：${stemRelation}`,
    suitable,
    avoid,
    luckyDirection: ELEMENT_DIRECTION[usefulElement],
    luckyColor: ELEMENT_COLOR[usefulElement],
    luckyNumbers: ELEMENT_NUMBERS[usefulElement],
    usefulElement,
    marketAdvice,
    sectorHint,
    poem,
  };
}
