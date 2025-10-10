/// <reference types="vite/client" />

// Типы для markdown файлов
declare module '*.md' {
  const content: string
  export default content
}

// Типы для vite-plugin-markdown
declare module '*.md?raw' {
  const content: string
  export default content
}