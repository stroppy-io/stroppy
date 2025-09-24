const { Project, SourceFile } = require("ts-morph");

const project = new Project();
project.addSourceFilesAtPaths("./ts_sdk/**/*.ts");
const combinedFile = project.createSourceFile("stroppy.pb.ts", undefined, { overwrite: true });
for (const sourceFile of project.getSourceFiles()) {
    if (!sourceFile.isDeclarationFile()) {
        // Собираем все import'ы
        sourceFile.getImportDeclarations().forEach(imp => {
            if (imp.getText().includes('from "./')) {
                imp.remove();
            }
        });
        if (sourceFile.getFilePath().includes('/gen/')) {
            console.log("skipping: ", sourceFile.getFilePath());
            continue;
        }
        combinedFile.addStatements(sourceFile.getFullText().trim());
    }
}
combinedFile.organizeImports();
combinedFile.saveSync();
console.log("stroppy.pb.ts");