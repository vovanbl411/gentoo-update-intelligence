# Frontend анализа

Frontend находится на review. Результаты реальной браузерной проверки и её ограничения описаны в [acceptance.md](acceptance.md).

Для локального запуска нужны Node.js, npm и запущенный backend, настроенный по [контракту HTTP API](../docs/http-api.md). Из корня репозитория:

```sh
npm ci --prefix frontend
npm run dev --prefix frontend
```

Откройте адрес, выведенный Vite. Запросы `/api` направляются на `http://127.0.0.1:8080`, при этом префикс удаляется. Для другого адреса backend измените локальную конфигурацию proxy. Будущий сервер публикации должен отдавать HTML для маршрутов приложения и проксировать `/api`, не подменяя ошибки API HTML-страницей.

Проверки:

```sh
npm run typecheck --prefix frontend
npm test --prefix frontend
npm run test:browser --prefix frontend
npm run build --prefix frontend
```

Для browser tests нужен установленный браузер Playwright. Информация о пакетах, findings, основаниях и metadata выводится из API; исходный текст плана при повторном открытии не восстанавливается. Отсутствие metadata не означает отсутствие изменений.
