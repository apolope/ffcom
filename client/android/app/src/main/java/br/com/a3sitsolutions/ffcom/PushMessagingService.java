package br.com.a3sitsolutions.ffcom;

import android.graphics.Bitmap;
import androidx.annotation.NonNull;
import com.google.firebase.messaging.FirebaseMessagingService;
import com.google.firebase.messaging.RemoteMessage;

// Recebe as mensagens do FCM (fase 6). O server-central manda só dados (sem o
// campo "notification"), então o Android não mostra nada sozinho e esta
// classe decide tudo, com o app aberto ou fechado: o formato (PushPayload),
// o que a pessoa já está vendo (PushState) e o agrupamento por canal
// (PushNotifier). É o único FirebaseMessagingService do app: o
// @capacitor/push-notifications não entra, porque traria um segundo serviço
// para o mesmo MESSAGING_EVENT (ver docs/architecture.md, "Decisão:
// notificações push no app Android (fase 6, client)").
public class PushMessagingService extends FirebaseMessagingService {

    @Override
    public void onMessageReceived(@NonNull RemoteMessage message) {
        PushPayload payload = PushPayload.parse(message.getData());
        if (payload == null || PushState.suppress(payload)) return;
        // Avatar de quem mandou: do cache ou baixado agora, com prazo de
        // 5 s (PushAvatars). Fora do lock do PushNotifier, para um download
        // lento não segurar outra notificação. null: fica a letra.
        Bitmap avatar = PushAvatarImages.load(this, payload);
        PushNotifier.show(this, payload, avatar);
    }

    // O FCM trocou o token do aparelho. Com a página aberta, ela registra o
    // novo no server-central na hora; sem ela, na próxima abertura (o
    // getToken já devolve o novo).
    @Override
    public void onNewToken(@NonNull String token) {
        PushPlugin.onNewToken(token);
    }
}
