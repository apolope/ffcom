// Idioma do site. Lê locales/<idioma>.json (o mesmo arquivo do client e dos
// servidores, copiado da raiz no build da imagem) e traduz a página. Ver
// docs/architecture.md, "Decisão: internacionalização".
//
// Carregado sem `defer` no <head>, antes dos outros scripts: escolhe o
// idioma (localStorage, depois navigator.languages: pt* → pt-BR, o resto →
// en) e já começa o fetch. O HTML vem em português; se o idioma for outro,
// a classe `i18n-pendente` no <html> esconde a página (styles.css) até a
// tradução ser aplicada, para não piscar o texto em português. Se o arquivo
// não chegar em 3 segundos ou falhar, a página aparece em português.
//
// Marcação:
//   data-i18n="chave"                  texto do elemento
//   data-i18n-html="chave"             HTML do elemento (só textos do próprio
//                                      locales/, com link ou negrito no meio)
//   data-i18n-attr="alt:chave;title:chave"  atributos
//   data-i18n-lang="en"                botão que troca o idioma (fica dentro
//                                      de um [data-i18n-idiomas] escondido
//                                      até o script rodar)
//
// API em window.ffcomI18n:
//   t(chave, params)       texto com {{x}} e plural (params.count, sufixos
//                          _one/_other/_many, por Intl.PluralRules, caindo em
//                          _other), como o i18next. Sem chave, devolve a chave.
//   exists(chave)          se a chave existe no idioma ativo
//   errorText(problema)    erro da API {code, message, params}: t('errors.'
//                          + code, params) ou, sem tradução, a message
//   lang                   idioma ativo ('pt-BR' ou 'en')
//   ready                  Promise resolvida com o arquivo carregado e a
//                          página traduzida
//   setLanguage(lng)       troca, grava no localStorage e retraduz sem
//                          recarregar
//   onChange(fn)           chamado depois de cada troca, com o idioma novo
//   apply(raiz)            traduz a marcação dentro de raiz (padrão: document)
(() => {
  const IDIOMAS = ['pt-BR', 'en'];
  const BASE = 'pt-BR';
  const CHAVE_STORAGE = 'ffcom.language';
  const raiz = document.documentElement;

  const normalizar = (lng) => (/^pt\b/i.test(lng || '') ? 'pt-BR' : 'en');

  const salvo = () => {
    try {
      const v = localStorage.getItem(CHAVE_STORAGE);
      return IDIOMAS.includes(v) ? v : null;
    } catch {
      return null;
    }
  };

  const doNavegador = () => {
    const lista = navigator.languages?.length ? navigator.languages : [navigator.language];
    return normalizar(lista.find(Boolean));
  };

  let lang = salvo() || doNavegador();
  let dic = {};
  const ouvintes = [];
  const cache = {};

  raiz.lang = lang;
  if (lang !== BASE) raiz.classList.add('i18n-pendente');
  const mostrar = () => raiz.classList.remove('i18n-pendente');
  const failsafe = setTimeout(mostrar, 3000);

  const carregar = (lng) => {
    cache[lng] ??= fetch(`/locales/${lng}.json`).then((r) => {
      if (!r.ok) throw new Error(`HTTP ${r.status}`);
      return r.json();
    }).catch((e) => {
      delete cache[lng];
      throw e;
    });
    return cache[lng];
  };

  const buscar = (chave) => {
    let no = dic;
    for (const parte of chave.split('.')) {
      if (no == null || typeof no !== 'object') return undefined;
      no = no[parte];
    }
    return typeof no === 'string' ? no : undefined;
  };

  const regras = {};
  const plural = (n) => {
    regras[lang] ??= new Intl.PluralRules(lang);
    return regras[lang].select(n);
  };

  const t = (chave, params = {}) => {
    let texto;
    if (typeof params.count === 'number') {
      texto = buscar(`${chave}_${plural(params.count)}`) ?? buscar(`${chave}_other`);
    }
    texto ??= buscar(chave);
    if (texto === undefined) return chave;
    return texto.replace(/\{\{\s*([\w.]+)\s*\}\}/g, (todo, nome) => (params[nome] ?? todo));
  };

  const exists = (chave) => buscar(chave) !== undefined || buscar(`${chave}_other`) !== undefined;

  const errorText = (problema) => {
    if (!problema) return '';
    if (problema.code && exists(`errors.${problema.code}`)) return t(`errors.${problema.code}`, problema.params || {});
    return problema.message || '';
  };

  const apply = (alvo = document) => {
    if (!Object.keys(dic).length) return;
    for (const el of alvo.querySelectorAll('[data-i18n]')) {
      const chave = el.dataset.i18n;
      if (exists(chave)) el.textContent = t(chave);
    }
    for (const el of alvo.querySelectorAll('[data-i18n-html]')) {
      const chave = el.dataset.i18nHtml;
      if (exists(chave)) el.innerHTML = t(chave);
    }
    for (const el of alvo.querySelectorAll('[data-i18n-attr]')) {
      for (const par of el.dataset.i18nAttr.split(';')) {
        const [attr, chave] = par.split(':').map((s) => s.trim());
        if (attr && chave && exists(chave)) el.setAttribute(attr, t(chave));
      }
    }
    for (const b of alvo.querySelectorAll('[data-i18n-lang]')) {
      b.setAttribute('aria-pressed', String(b.dataset.i18nLang === lang));
    }
  };

  const domPronto = new Promise((r) => {
    if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', r, { once: true });
    else r();
  });

  // Liga os botões de idioma do rodapé.
  domPronto.then(() => {
    for (const grupo of document.querySelectorAll('[data-i18n-idiomas]')) grupo.hidden = false;
    document.addEventListener('click', (ev) => {
      const b = ev.target.closest?.('[data-i18n-lang]');
      if (b) setLanguage(b.dataset.i18nLang);
    });
  });

  // Primeiro carregamento: o idioma escolhido, ou o português se ele falhar.
  const ready = carregar(lang)
    .catch((e) => {
      console.warn(`Idioma ${lang} indisponível:`, e);
      if (lang === BASE) throw e;
      lang = BASE;
      raiz.lang = lang;
      return carregar(BASE);
    })
    .then((d) => { dic = d; })
    .catch((e) => console.warn('Arquivo de idioma indisponível; a página fica no texto do HTML.', e))
    .then(() => domPronto)
    .then(() => {
      apply();
      clearTimeout(failsafe);
      mostrar();
    });

  const setLanguage = async (lng) => {
    lng = IDIOMAS.includes(lng) ? lng : normalizar(lng);
    try { localStorage.setItem(CHAVE_STORAGE, lng); } catch { /* sem storage */ }
    if (lng === lang) return;
    try {
      dic = await carregar(lng);
    } catch (e) {
      console.warn(`Idioma ${lng} indisponível:`, e);
      return;
    }
    lang = lng;
    raiz.lang = lng;
    apply();
    for (const fn of ouvintes) {
      try { fn(lng); } catch (e) { console.warn(e); }
    }
  };

  window.ffcomI18n = {
    t,
    exists,
    errorText,
    get lang() { return lang; },
    ready,
    setLanguage,
    onChange: (fn) => { ouvintes.push(fn); },
    apply,
  };
})();
