const path = require('path');
const {glob} = require("glob");


module.exports = {
    entry: "./stroppy.pb.ts",         // Главный файл (точка входа)
    mode: 'production',              // Или 'development' для отладки
    module: {
        rules: [
            {
                test: /\.tsx?$/,
                use: {
                    loader: 'ts-loader',
                    options: { transpileOnly: true }
                },          // Загрузчик для TypeScript
                exclude: /node_modules/,
            },
        ],
    },
    resolve: {
        extensions: ['.tsx', '.ts', '.js'],  // Разрешает импорт без указания расширений
    },
    externalsType: 'commonjs',
    externals: [/^k6(\/.*)?$/],
    output: {
        filename: 'bundle.js',         // Итоговый файл
        path: path.resolve(__dirname, 'dist'),
        libraryTarget: 'umd',          // Универсальный модуль (поддержка CommonJS, AMD и глобальной переменной)
        globalObject: 'this',          // Для корректной работы в браузере и Node.js
    },
};