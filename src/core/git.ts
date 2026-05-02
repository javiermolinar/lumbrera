export interface GitCommandResult {
  stdout: string;
  stderr: string;
}

export async function git(args: string[], options: { cwd: string }): Promise<GitCommandResult> {
  void args;
  void options;
  throw new Error('git wrapper is not implemented yet');
}
