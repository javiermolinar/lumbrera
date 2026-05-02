export interface GeneratedFiles {
  index: string;
  changelog: string;
  brainSum: string;
}

export async function generateFiles(repo: string): Promise<GeneratedFiles> {
  void repo;
  throw new Error('generated files are not implemented yet');
}
