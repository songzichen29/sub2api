export const timezone = 'Asia/Shanghai';
const formatter = new Intl.DateTimeFormat('sv-SE', {
  timeZone: timezone, year: 'numeric', month: '2-digit', day: '2-digit',
  hour: '2-digit', minute: '2-digit', second: '2-digit', hourCycle: 'h23'
});

// Storage stays in UTC. Only the view supplied to the model is localized.
export function beijingTime(value = Date.now()) {
  const date = new Date(value);
  return Number.isFinite(date.getTime()) ? formatter.format(date).replace(' ', 'T') + '+08:00' : '';
}
export const localRecords = records => records.map(row => ({ ...row, at: beijingTime(row.at) }));
export const timeGuidance = '\n时间规则：本机器人与群成员默认使用北京时间 Asia/Shanghai（UTC+8）。每轮 time 是当前北京时间；聊天记录 at 已转换为带 +08:00 的北京时间，不得再次加八小时。未明确标注其他时区的“凌晨两点”“今天”“明天”等均按北京时间理解，不能把用户说的凌晨两点改成上午十点。历史 UTC/Z 只表示存储格式，不是群成员所在时区；旧回复若换算有误，以本规则和本轮 time 为准。';
