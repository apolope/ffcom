package br.com.a3sitsolutions.ffcom;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertNotNull;
import static org.junit.Assert.assertNull;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

public class CallNetworkTest {

    private static final long WIFI = 101;
    private static final long LTE = 202;

    @Test
    public void primeiraRedeEPontoDePartida() {
        CallNetwork n = new CallNetwork();
        assertNull(n.onDefault(WIFI, "wifi"));
        // Mudança de sinal ou banda na mesma rede não é troca.
        assertNull(n.onDefault(WIFI, "wifi"));
    }

    @Test
    public void trocaDeWifiParaDadosMoveisEVolta() {
        CallNetwork n = new CallNetwork();
        n.onDefault(WIFI, "wifi");
        // O Android Auto sem fio toma o Wi-Fi: o Wi-Fi cai e os dados móveis
        // assumem.
        CallNetwork.Event lost = n.onLost(WIFI);
        assertNotNull(lost);
        assertFalse(lost.available);
        assertEquals("none", lost.transport);
        CallNetwork.Event lte = n.onDefault(LTE, "cellular");
        assertNotNull(lte);
        assertTrue(lte.available);
        assertEquals("cellular", lte.transport);
        // Wi-Fi de volta, direto como nova rede padrão.
        CallNetwork.Event back = n.onDefault(WIFI, "wifi");
        assertNotNull(back);
        assertEquals("wifi", back.transport);
    }

    @Test
    public void perderOutraRedeNaoMudaNada() {
        CallNetwork n = new CallNetwork();
        assertNull(n.onLost(WIFI));
        n.onDefault(LTE, "cellular");
        assertNull(n.onLost(WIFI));
        assertNull(n.onDefault(LTE, "cellular"));
    }

    @Test
    public void nomeDoTransporte() {
        assertEquals("vpn", CallNetwork.transportName(true, true, false, false));
        assertEquals("wifi", CallNetwork.transportName(false, true, false, false));
        assertEquals("cellular", CallNetwork.transportName(false, false, true, false));
        assertEquals("ethernet", CallNetwork.transportName(false, false, false, true));
        assertEquals("other", CallNetwork.transportName(false, false, false, false));
    }
}
