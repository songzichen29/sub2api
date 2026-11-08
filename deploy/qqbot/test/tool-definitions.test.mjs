import test from 'node:test';
import assert from 'node:assert/strict';
import { validateArgs } from '@deepseek-ai/dsh-tools';
import { apply } from '../src/dsh-tools-plugin.mjs';
import { toolDefinitions } from '../src/tool-definitions.mjs';

test('every QQ tool registers with the pinned SDK and image formats stay optional', () => {
  const registered = [];
  apply({ tools: { register(tool) { registered.push(tool); } } });
  assert.deepEqual(registered.map(tool => tool.name), toolDefinitions.map(tool => tool.name));
  assert.ok(registered.some(tool => tool.name === 'send_meme'));
  for (const name of ['generate_image', 'edit_image']) {
    const definition = toolDefinitions.find(tool => tool.name === name);
    const tool = registered.find(tool => tool.name === name);
    const args = { prompt: '画一只猫', ...(name === 'edit_image' ? { image_index: 1 } : {}) };
    assert.equal(tool.parameters.required.includes('output_format'), false);
    assert.deepEqual(validateArgs(definition.parameters, args), []);
    assert.deepEqual(validateArgs(definition.parameters, { ...args, output_format: 'gif' }), []);
    assert.notDeepEqual(validateArgs(definition.parameters, { ...args, output_format: 1 }), []);
  }
});
