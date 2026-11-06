const analysesPath = '/api/v1/analyses';

export interface CreateAnalysisRequest {
  plan: string;
  locally_patched_packages: { category: string; name: string }[];
}

export function createAnalysis(body: CreateAnalysisRequest): Promise<Response> {
  return fetch(analysesPath, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
}

export function getAnalysis(id: string): Promise<Response> {
  return fetch(`${analysesPath}/${encodeURIComponent(id)}`);
}

export function getAnalysisReport(id: string): Promise<Response> {
  return fetch(`${analysesPath}/${encodeURIComponent(id)}/report`);
}

export function getPartialReport(id: string): Promise<Response> {
  return fetch(`${analysesPath}/${encodeURIComponent(id)}/partial-report`);
}

// Codes are stable identifiers; even a code unknown to this frontend retains
// its message. Rendering uses textContent, never HTML from the API.
export async function responseErrorMessage(response: Response): Promise<string> {
  try {
    const data: unknown = await response.json();
    if (typeof data !== 'object' || data === null || !('error' in data)) return '';
    const error = data.error;
    if (typeof error !== 'object' || error === null || !('message' in error)) return '';
    return typeof error.message === 'string' ? error.message : '';
  } catch {
    return '';
  }
}
