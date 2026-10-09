package br.com.a3sitsolutions.ffcom;

// Regra pura do acompanhamento da rede durante a chamada (CallService), à
// parte para os testes JUnit em app/src/test. O CallService registra um
// ConnectivityManager.NetworkCallback na rede padrão e passa cada callback
// para cá; o que sai daqui (não null) vira o evento "network" do CallPlugin,
// {available, transport}, para a página reconectar a chamada na hora em vez
// de esperar o livekit-client perceber sozinho. Ver docs/architecture.md,
// "Decisão: chamada em segundo plano no Android (fase 5)".
final class CallNetwork {

    // O que a página recebe.
    static final class Event {

        final boolean available;
        // "wifi", "cellular", "ethernet", "vpn", "other" ou "none".
        final String transport;

        Event(boolean available, String transport) {
            this.available = available;
            this.transport = transport;
        }
    }

    // Rede padrão atual (Network.getNetworkHandle; 0 sem rede) e o
    // transporte dela; baseline diz se o primeiro callback, que só descreve
    // a rede de quando o registro foi feito, já chegou.
    private long network;
    private String transport = "none";
    private boolean baseline;

    // onCapabilitiesChanged da rede padrão (sempre vem depois de onAvailable,
    // e de novo a cada mudança de sinal ou banda). Só uma rede padrão nova ou
    // um transporte novo contam como troca; o primeiro é a rede de partida.
    synchronized Event onDefault(long handle, String nextTransport) {
        boolean first = !baseline;
        boolean changed = handle != network || !nextTransport.equals(transport);
        baseline = true;
        network = handle;
        transport = nextTransport;
        if (first || !changed) return null;
        return new Event(true, nextTransport);
    }

    // onLost: a rede padrão sumiu sem outra no lugar (com outra, o Android
    // manda direto o onAvailable/onCapabilitiesChanged da nova). Perder uma
    // rede que não é a atual não muda nada.
    synchronized Event onLost(long handle) {
        if (!baseline || handle != network) return null;
        network = 0;
        transport = "none";
        return new Event(false, "none");
    }

    // Nome do transporte a partir das NetworkCapabilities. VPN primeiro: por
    // cima dela o transporte de baixo também aparece, e a troca dele já
    // chega como mudança de capacidades da própria VPN.
    static String transportName(boolean vpn, boolean wifi, boolean cellular, boolean ethernet) {
        if (vpn) return "vpn";
        if (wifi) return "wifi";
        if (cellular) return "cellular";
        if (ethernet) return "ethernet";
        return "other";
    }
}
