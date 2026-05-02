#!/usr/bin/env node

import { init } from './core/init.js';
import { sync } from './core/sync.js';
import { write } from './core/write.js';

async function main(argv: string[]): Promise<void> {
  const [command, ...args] = argv;

  switch (command) {
    case 'init':
      await init(args);
      return;
    case 'sync':
      await sync(args);
      return;
    case 'write':
      await write(args, process.stdin);
      return;
    default:
      printUsage();
      process.exitCode = command ? 1 : 0;
  }
}

function printUsage(): void {
  console.log(`Usage: lumbrera <command> [options]

Commands:
  init <repo>              Create the minimal Lumbrera brain scaffold
  sync --repo <repo>       Converge repo to a valid Lumbrera state
  write <path> [options]   Perform one atomic mutation transaction
`);
}

main(process.argv.slice(2)).catch((error) => {
  console.error(error instanceof Error ? error.message : String(error));
  process.exitCode = 1;
});
