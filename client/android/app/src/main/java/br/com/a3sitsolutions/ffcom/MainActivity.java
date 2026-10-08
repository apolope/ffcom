package br.com.a3sitsolutions.ffcom;

import android.os.Bundle;
import com.getcapacitor.BridgeActivity;

public class MainActivity extends BridgeActivity {

    @Override
    public void onCreate(Bundle savedInstanceState) {
        // Plugins próprios do app (no Capacitor 8, registrados antes do
        // super.onCreate, que monta a ponte). UpdatePlugin: atualização do APK.
        registerPlugin(UpdatePlugin.class);
        super.onCreate(savedInstanceState);
    }
}
