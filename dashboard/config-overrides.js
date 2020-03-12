/*eslint-disable no-unused-vars*/

const MiniCssExtractPlugin = require('mini-css-extract-plugin');

// this tweaks the webpack configuration, thanks to react-app-rewired
module.exports = function override(config, env) {
    config.output.filename = 'static/js/[name].js';
    config.output.chunkFilename = 'static/js/[name].js';
    config.optimization.splitChunks = false;
    config.optimization.runtimeChunk = false;

    config.plugins.forEach((plugin) => {
        if (plugin instanceof MiniCssExtractPlugin) {
            plugin.options.filename = 'static/css/[name].css';
        }
    });

    return config;
};
