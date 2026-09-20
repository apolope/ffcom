// Cliente HTTP para a API de server-central: diretório de server-channel
// conhecidos pela conta autenticada. Ver docs/architecture.md, "Decisão:
// descoberta de server-channel" — sem descoberta automática, só convite ou
// endereço adicionado manualmente.

// Instância oficial única do server-central (ver docs/architecture.md,
// contexto: "server-central (instância única oficial)"). Domínio de
// produção ainda não provisionado (ver TODO.md); em dev local aponta para o
// docker-compose de referência (SERVER_CENTRAL_PORT=8081).
export const SERVER_CENTRAL_URL: string =
  import.meta.env.VITE_SERVER_CENTRAL_URL ?? 'http://localhost:8081'

export interface RemoteKnownServer {
  id: string
  address: string
  name: string
  iconUrl?: string
  addedAt: string
}

async function parseJsonOrThrow<T>(res: Response): Promise<T> {
  if (!res.ok) {
    throw new Error(`server-central: ${res.status} ${res.statusText}`)
  }
  return res.json() as Promise<T>
}

export async function fetchKnownServers(accessToken: string): Promise<RemoteKnownServer[]> {
  const res = await fetch(`${SERVER_CENTRAL_URL}/api/servers`, {
    headers: { Authorization: `Bearer ${accessToken}` },
  })
  const body = await parseJsonOrThrow<{ servers: RemoteKnownServer[] }>(res)
  return body.servers
}

export async function addKnownServer(
  accessToken: string,
  address: string,
  name: string,
): Promise<RemoteKnownServer> {
  const res = await fetch(`${SERVER_CENTRAL_URL}/api/servers`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${accessToken}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ address, name }),
  })
  return parseJsonOrThrow<RemoteKnownServer>(res)
}

export async function removeKnownServer(accessToken: string, id: string): Promise<void> {
  const res = await fetch(`${SERVER_CENTRAL_URL}/api/servers/${id}`, {
    method: 'DELETE',
    headers: { Authorization: `Bearer ${accessToken}` },
  })
  if (!res.ok) {
    throw new Error(`server-central: ${res.status} ${res.statusText}`)
  }
}
