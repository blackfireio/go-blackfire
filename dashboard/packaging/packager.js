#!/usr/bin/env node

const fs = require('fs');

const dir = `${__dirname}/..`;

fs.mkdirSync(`${dir}/dist`, { recursive: true });

fs.writeFileSync(`${dir}/dist/index.html`, fs.readFileSync(`${dir}/build/index.html`, { encoding: 'utf8' }).replace('<link href="/static/css/main.css" rel="stylesheet">', '').replace('<script src="/static/js/main.js"></script>', () => `<script>${fs.readFileSync(`${dir}/build/merged/bundle.js`, { encoding: 'utf8' })}</script>`));
