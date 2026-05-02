import type { Readable } from 'node:stream';

export async function write(args: string[], stdin: Readable): Promise<void> {
  void args;
  void stdin;
  throw new Error('lumbrera write is not implemented yet');
}
