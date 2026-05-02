export interface ManifestEntry {
  path: string;
  hash: string;
}

export function generateManifest(entries: ManifestEntry[]): string {
  void entries;
  throw new Error('manifest generation is not implemented yet');
}
