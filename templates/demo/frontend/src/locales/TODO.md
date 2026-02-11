# Locale Expansion TODO

Current supported locales:
- `en`, `es`, `fr`, `de`, `pt`, `ja`, `zh-CN`, `zh-TW`, `ko-KR`, `it-IT`, `nl-NL`, `ru-RU`, `pl-PL`, `tr-TR`, `id-ID`, `vi-VN`, `th-TH`

Suggested next languages to add:

## High Priority
- [ ] `ar` (Arabic)
- [ ] `hi` (Hindi)
- [ ] `bn` (Bengali)
- [ ] `ur` (Urdu)

## Medium Priority
- [ ] `uk` (Ukrainian)
- [ ] `ro` (Romanian)
- [ ] `sv` (Swedish)
- [ ] `cs` (Czech)

## Also Worth Considering
- [ ] `el` (Greek)
- [ ] `he` (Hebrew)
- [ ] `fa` (Persian)
- [ ] `ms` (Malay)

Notes:
- Add locale entries in `src/i18n.ts` (`LOCALES`, imports, `LOCALE_OPTIONS`, detection, messages).
- Add language labels for new keys in every locale file under `language.*`.
- Keep brand name `Uptime Gopher` untranslated.
