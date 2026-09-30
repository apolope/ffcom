// Separa o link de convite gerado por InviteServerDialog (endereço do
// server-channel + `?invite=CODE`, e `&name=` desde o convite auto-contido)
// de volta em endereço, código e nome. Devolve undefined se o valor não for
// uma URL com `invite`. Usado pelo AddServerDialog e pelo NotMemberPanel.
// Ver docs/architecture.md, "Decisão: convite auto-contido".
export function parseInviteLink(value: string): { address: string; inviteCode: string; name?: string } | undefined {
  let url: URL
  try {
    url = new URL(value)
  } catch {
    return undefined
  }
  const code = url.searchParams.get('invite')
  if (!code) return undefined
  // Links anteriores ao `&name=` não trazem o nome; a pessoa digita.
  const name = url.searchParams.get('name')?.trim() || undefined
  url.search = ''
  return { address: url.toString().replace(/\/+$/, ''), inviteCode: code, name }
}
