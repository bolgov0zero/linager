import { defineConfig, type DefaultTheme } from 'vitepress';

const REPO = 'https://github.com/bolgov0zero/linager';
const SITE = 'https://bolgov0zero.github.io/linager';

const sidebar = (p: string, t: Record<string, string>): DefaultTheme.SidebarItem[] => [
  { text: t.start, items: [
    { text: t.introduction, link: `${p}/introduction` },
    { text: t.install, link: `${p}/install` },
  ] },
  { text: t.guides, items: [
    { text: t.proxy, link: `${p}/proxy` },
    { text: t.access, link: `${p}/access` },
    { text: t.connection, link: `${p}/connection` },
    { text: t.modules, link: `${p}/modules` },
    { text: t.menu, link: `${p}/menu` },
    { text: t.console, link: `${p}/console` },
    { text: t.keys, link: `${p}/keys` },
    { text: t.windows, link: `${p}/windows` },
  ] },
  { text: t.ops, items: [
    { text: t.updates, link: `${p}/updates` },
    { text: t.security, link: `${p}/security` },
  ] },
  { text: t.ref, items: [{ text: t.architecture, link: `${p}/architecture` }] },
];

const docsMatch = (p: string) => `^${p}/(introduction|install|proxy|access|connection|modules|menu|console|keys|windows|updates|security|architecture)`;

export default defineConfig({
  srcDir: 'content',
  title: 'Linager',
  description: 'Open-source server management for Debian and Ubuntu: a web panel and lightweight agents.',
  base: '/linager/',
  cleanUrls: true,
  lastUpdated: false,
  head: [
    ['link', { rel: 'icon', type: 'image/svg+xml', href: '/linager/logo.svg' }],
    ['meta', { property: 'og:type', content: 'website' }],
    ['meta', { property: 'og:title', content: 'Linager: server management for Debian and Ubuntu' }],
    ['meta', { property: 'og:description', content: 'One web panel, a lightweight agent on every host: live metrics, terminal, files, firewall, Docker.' }],
    ['meta', { property: 'og:image', content: `${SITE}/screenshots/en/dashboard.png` }],
    ['meta', { name: 'theme-color', content: '#2b55d9' }],
  ],
  locales: {
    root: {
      label: 'English',
      lang: 'en-US',
      themeConfig: {
        nav: [
          { text: 'Home', link: '/', activeMatch: '^/$' },
          { text: 'Documentation', link: '/introduction', activeMatch: docsMatch('') },
          { text: 'Releases', link: `${REPO}/releases` },
        ],
        sidebar: sidebar('', {
          start: 'Getting started', guides: 'Guides', ops: 'Operations', ref: 'Reference',
          introduction: 'Introduction', install: 'Installation', proxy: 'Reverse proxy', access: 'Admin panel access', connection: 'Agent and application ports',
          modules: 'Agent modules', menu: 'Terminal menu', console: 'The Console view', keys: 'Keys', windows: 'Windows agent', updates: 'Updates and backups', security: 'Security', architecture: 'Architecture',
        }),
        outline: { label: 'On this page', level: [2, 3] },
        docFooter: { prev: 'Previous', next: 'Next' },
        footer: { message: 'Released under the Apache-2.0 license. Fonts: Medeo, SIL OFL 1.1.', copyright: 'Linager' },
      },
    },
    ru: {
      label: 'Русский',
      lang: 'ru-RU',
      link: '/ru/',
      description: 'Открытая система управления серверами на Debian и Ubuntu: веб-админка и лёгкие агенты.',
      themeConfig: {
        nav: [
          { text: 'Главная', link: '/ru/', activeMatch: '^/ru/$' },
          { text: 'Документация', link: '/ru/introduction', activeMatch: docsMatch('/ru') },
          { text: 'Релизы', link: `${REPO}/releases` },
        ],
        sidebar: sidebar('/ru', {
          start: 'Начало работы', guides: 'Руководства', ops: 'Эксплуатация', ref: 'Справочник',
          introduction: 'Введение', install: 'Установка', proxy: 'Обратный прокси', access: 'Доступ к админке', connection: 'Порты агентов и приложения',
          modules: 'Модули агента', menu: 'Меню в терминале', console: 'Вид «Консоль»', keys: 'Ключи', windows: 'Агент для Windows', updates: 'Обновления и бэкапы', security: 'Безопасность', architecture: 'Архитектура',
        }),
        outline: { label: 'На этой странице', level: [2, 3] },
        docFooter: { prev: 'Назад', next: 'Дальше' },
        darkModeSwitchLabel: 'Тема',
        sidebarMenuLabel: 'Меню',
        returnToTopLabel: 'Наверх',
        langMenuLabel: 'Язык',
        lightModeSwitchTitle: 'Светлая тема',
        darkModeSwitchTitle: 'Тёмная тема',
        footer: { message: 'Лицензия Apache-2.0. Шрифты: Medeo, SIL OFL 1.1.', copyright: 'Linager' },
      },
    },
  },
  themeConfig: {
    logo: '/logo.svg',
    socialLinks: [{ icon: 'github', link: REPO }],
    search: {
      provider: 'local',
      options: {
        locales: {
          ru: {
            translations: {
              button: { buttonText: 'Поиск', buttonAriaLabel: 'Поиск' },
              modal: {
                displayDetails: 'Показать подробности',
                resetButtonTitle: 'Сбросить',
                backButtonTitle: 'Закрыть',
                noResultsText: 'Ничего не найдено',
                footer: { selectText: 'выбрать', navigateText: 'перемещаться', closeText: 'закрыть' },
              },
            },
          },
        },
      },
    },
  },
});
