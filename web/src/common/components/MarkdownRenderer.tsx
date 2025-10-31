import React from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import rehypeHighlight from 'rehype-highlight'
import { Typography, Divider } from 'antd'
import { useTheme } from '../contexts/ThemeContext'
import 'highlight.js/styles/github.css'
import 'highlight.js/styles/github-dark.css'

const { Title, Paragraph, Text } = Typography

interface MarkdownRendererProps {
  content: string
  className?: string
}

const MarkdownRenderer: React.FC<MarkdownRendererProps> = ({ content, className }) => {
  const { darkReaderEnabled } = useTheme()

  return (
    <div className={className}>
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        rehypePlugins={[rehypeHighlight]}
        components={{
          h1: ({ children }) => (
            <Title level={1} style={{ 
              color: darkReaderEnabled ? '#ffffff' : '#262626',
              marginTop: '24px',
              marginBottom: '16px'
            }}>
              {children}
            </Title>
          ),
          h2: ({ children }) => (
            <Title level={2} style={{ 
              color: darkReaderEnabled ? '#ffffff' : '#262626',
              marginTop: '20px',
              marginBottom: '12px'
            }}>
              {children}
            </Title>
          ),
          h3: ({ children }) => (
            <Title level={3} style={{ 
              color: darkReaderEnabled ? '#ffffff' : '#262626',
              marginTop: '16px',
              marginBottom: '8px'
            }}>
              {children}
            </Title>
          ),
          h4: ({ children }) => (
            <Title level={4} style={{ 
              color: darkReaderEnabled ? '#ffffff' : '#262626',
              marginTop: '12px',
              marginBottom: '6px'
            }}>
              {children}
            </Title>
          ),
          h5: ({ children }) => (
            <Title level={5} style={{ 
              color: darkReaderEnabled ? '#ffffff' : '#262626',
              marginTop: '10px',
              marginBottom: '4px'
            }}>
              {children}
            </Title>
          ),
          h6: ({ children }) => (
            <Title level={5} style={{ 
              color: darkReaderEnabled ? '#ffffff' : '#262626',
              marginTop: '8px',
              marginBottom: '4px',
              fontSize: '14px'
            }}>
              {children}
            </Title>
          ),
          p: ({ children }) => (
            <Paragraph style={{ 
              color: darkReaderEnabled ? '#d9d9d9' : '#595959',
              marginBottom: '16px',
              lineHeight: '1.6'
            }}>
              {children}
            </Paragraph>
          ),
          code: ({ children, className }) => {
            const isInline = !className
            if (isInline) {
              return (
                <Text 
                  code 
                  style={{ 
                    background: darkReaderEnabled ? '#2a2a2a' : '#f5f5f5',
                    color: darkReaderEnabled ? '#40a9ff' : '#1890ff',
                    padding: '2px 6px',
                    borderRadius: '4px',
                    fontSize: '13px'
                  }}
                >
                  {children}
                </Text>
              )
            }
            return (
              <pre style={{
                background: darkReaderEnabled ? '#1a1a1a' : '#f6f8fa',
                border: darkReaderEnabled ? '1px solid #333' : '1px solid #e1e4e8',
                borderRadius: '6px',
                padding: '16px',
                overflow: 'auto',
                margin: '16px 0'
              }}>
                <code className={className}>{children}</code>
              </pre>
            )
          },
          pre: ({ children }) => (
            <div style={{
              background: darkReaderEnabled ? '#1a1a1a' : '#f6f8fa',
              border: darkReaderEnabled ? '1px solid #333' : '1px solid #e1e4e8',
              borderRadius: '6px',
              padding: '16px',
              overflow: 'auto',
              margin: '16px 0'
            }}>
              {children}
            </div>
          ),
          blockquote: ({ children }) => (
            <div style={{
              borderLeft: `4px solid ${darkReaderEnabled ? '#40a9ff' : '#1890ff'}`,
              paddingLeft: '16px',
              margin: '16px 0',
              background: darkReaderEnabled ? 'rgba(64, 169, 255, 0.1)' : 'rgba(24, 144, 255, 0.1)',
              padding: '12px 16px',
              borderRadius: '4px'
            }}>
              {children}
            </div>
          ),
          ul: ({ children }) => (
            <ul style={{ 
              color: darkReaderEnabled ? '#d9d9d9' : '#595959',
              marginBottom: '16px',
              paddingLeft: '24px'
            }}>
              {children}
            </ul>
          ),
          ol: ({ children }) => (
            <ol style={{ 
              color: darkReaderEnabled ? '#d9d9d9' : '#595959',
              marginBottom: '16px',
              paddingLeft: '24px'
            }}>
              {children}
            </ol>
          ),
          li: ({ children }) => (
            <li style={{ marginBottom: '4px' }}>
              {children}
            </li>
          ),
          table: ({ children }) => (
            <div style={{ 
              overflow: 'auto', 
              margin: '16px 0',
              border: darkReaderEnabled ? '1px solid #333' : '1px solid #e8e8e8',
              borderRadius: '6px'
            }}>
              <table style={{ 
                width: '100%',
                borderCollapse: 'collapse',
                background: darkReaderEnabled ? '#1f1f1f' : '#ffffff'
              }}>
                {children}
              </table>
            </div>
          ),
          thead: ({ children }) => (
            <thead style={{ 
              background: darkReaderEnabled ? '#2a2a2a' : '#fafafa'
            }}>
              {children}
            </thead>
          ),
          tbody: ({ children }) => (
            <tbody>
              {children}
            </tbody>
          ),
          tr: ({ children }) => (
            <tr style={{ 
              borderBottom: darkReaderEnabled ? '1px solid #333' : '1px solid #e8e8e8'
            }}>
              {children}
            </tr>
          ),
          th: ({ children }) => (
            <th style={{ 
              padding: '12px 16px',
              textAlign: 'left',
              fontWeight: 'bold',
              color: darkReaderEnabled ? '#ffffff' : '#262626',
              borderRight: darkReaderEnabled ? '1px solid #333' : '1px solid #e8e8e8'
            }}>
              {children}
            </th>
          ),
          td: ({ children }) => (
            <td style={{ 
              padding: '12px 16px',
              color: darkReaderEnabled ? '#d9d9d9' : '#595959',
              borderRight: darkReaderEnabled ? '1px solid #333' : '1px solid #e8e8e8'
            }}>
              {children}
            </td>
          ),
          hr: () => (
            <Divider style={{ 
              borderColor: darkReaderEnabled ? '#333' : '#e8e8e8',
              margin: '24px 0'
            }} />
          ),
          a: ({ children, href }) => (
            <a 
              href={href}
              style={{ 
                color: darkReaderEnabled ? '#40a9ff' : '#1890ff',
                textDecoration: 'none'
              }}
              onMouseEnter={(e) => e.currentTarget.style.textDecoration = 'underline'}
              onMouseLeave={(e) => e.currentTarget.style.textDecoration = 'none'}
            >
              {children}
            </a>
          ),
          strong: ({ children }) => (
            <Text strong style={{ 
              color: darkReaderEnabled ? '#ffffff' : '#262626'
            }}>
              {children}
            </Text>
          ),
          em: ({ children }) => (
            <Text italic style={{ 
              color: darkReaderEnabled ? '#d9d9d9' : '#595959'
            }}>
              {children}
            </Text>
          )
        }}
      >
        {content}
      </ReactMarkdown>
    </div>
  )
}

export default MarkdownRenderer
