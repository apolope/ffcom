package br.com.a3sitsolutions.ffcom;

import java.util.ArrayList;
import java.util.Collections;
import java.util.List;

// O que uma notificação agrupada já mostra: quantas mensagens chegaram desde
// que ela apareceu e as últimas linhas (autor e texto). Fica em
// SharedPreferences entre uma mensagem e outra, porque o processo pode morrer
// no meio; some quando a notificação sai da tela (ver PushNotifier). Formato
// próprio em texto, sem org.json, para a classe ser pura e testável no JUnit
// (PushGroupTest).
final class PushGroup {

    // Linhas guardadas; o MessagingStyle mostra as últimas que couberem.
    static final int MAX_LINES = 5;

    private static final char FIELD = '\u001F';
    private static final char RECORD = '\u001E';

    static final class Line {
        final String author;
        final String text;
        final long time;

        Line(String author, String text, long time) {
            this.author = clean(author);
            this.text = clean(text);
            this.time = time;
        }
    }

    int count;
    private final List<Line> lines;

    PushGroup() {
        this(0, new ArrayList<>());
    }

    private PushGroup(int count, List<Line> lines) {
        this.count = count;
        this.lines = lines;
    }

    void add(String author, String text, long time) {
        count++;
        lines.add(new Line(author, text, time));
        while (lines.size() > MAX_LINES) lines.remove(0);
    }

    List<Line> lines() {
        return Collections.unmodifiableList(lines);
    }

    String encode() {
        StringBuilder sb = new StringBuilder();
        sb.append(count);
        for (Line line : lines) {
            sb.append(RECORD).append(line.time).append(FIELD).append(line.author).append(FIELD).append(line.text);
        }
        return sb.toString();
    }

    // Texto estragado ou de outra versão vira um grupo vazio: no pior caso a
    // contagem recomeça.
    static PushGroup decode(String encoded) {
        if (encoded == null || encoded.isEmpty()) return new PushGroup();
        String[] records = encoded.split(String.valueOf(RECORD), -1);
        int count;
        try {
            count = Integer.parseInt(records[0]);
        } catch (NumberFormatException e) {
            return new PushGroup();
        }
        List<Line> lines = new ArrayList<>();
        for (int i = 1; i < records.length; i++) {
            String[] fields = records[i].split(String.valueOf(FIELD), -1);
            if (fields.length != 3) return new PushGroup();
            try {
                lines.add(new Line(fields[1], fields[2], Long.parseLong(fields[0])));
            } catch (NumberFormatException e) {
                return new PushGroup();
            }
        }
        while (lines.size() > MAX_LINES) lines.remove(0);
        return new PushGroup(Math.max(count, lines.size()), lines);
    }

    // Os separadores não podem aparecer dentro de um campo.
    private static String clean(String s) {
        if (s == null) return "";
        return s.replace(FIELD, ' ').replace(RECORD, ' ');
    }
}
