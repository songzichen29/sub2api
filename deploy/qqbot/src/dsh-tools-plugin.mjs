import { defineTool } from '@deepseek-ai/dsh-tools';
import { toolDefinitions } from './tool-definitions.mjs';

export const name='qqbot-capabilities';
export const inject=['tools'];
export function apply(ctx) {
  for (const definition of toolDefinitions) ctx.tools.register(defineTool({
    ...definition, output:{schema:{type:'string'},render:(_args,value)=>[{type:'text',text:value}]},
    timeoutMs:35000, isConcurrencySafe:()=>false,
    async execute(args, exec) {
      const response=await fetch(process.env.QQBOT_TOOL_URL,{
        method:'POST',headers:{'Content-Type':'application/json','x-tool-token':process.env.QQBOT_TOOL_TOKEN},
        body:JSON.stringify({name:definition.name,args,session:exec.agent?.session.id,call_id:String(exec.callId)}),
        signal:AbortSignal.any([exec.signal,AbortSignal.timeout(35000)])
      });
      const result=await response.json();
      if(!response.ok) throw new Error(result.error || 'QQ工具执行失败');
      return JSON.stringify(result);
    }
  }));
}
