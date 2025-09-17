import { useState } from 'react'
import { Button, Card, Layout, Typography, Space, Row, Col } from 'antd'
import { ThunderboltOutlined, CloudOutlined, SettingOutlined } from '@ant-design/icons'

const { Header, Content, Footer } = Layout
const { Title, Paragraph } = Typography

function App() {
  const [count, setCount] = useState(0)

  return (
    <Layout className="min-h-screen">
      <Header className="bg-white shadow-sm">
        <div className="flex items-center justify-between h-full px-6">
          <div className="flex items-center space-x-2">
            <CloudOutlined className="text-2xl text-blue-600" />
            <Title level={3} className="!mb-0 text-blue-600">
              Stroppy Cloud Panel
            </Title>
          </div>
          <Space>
            <Button icon={<SettingOutlined />}>Настройки</Button>
            <Button type="primary" icon={<ThunderboltOutlined />}>
              Запустить
            </Button>
          </Space>
        </div>
      </Header>
      
      <Content className="p-6">
        <Row gutter={[16, 16]}>
          <Col span={24}>
            <Card className="text-center">
              <Title level={1} className="!mb-4">
                Добро пожаловать в Stroppy Cloud Panel
              </Title>
              <Paragraph className="text-lg text-gray-600 mb-6">
                Современная панель управления облачными ресурсами
              </Paragraph>
              
              <div className="mb-6">
                <Button 
                  type="primary" 
                  size="large"
                  onClick={() => setCount((count) => count + 1)}
                  className="mr-4"
                >
                  Счетчик: {count}
                </Button>
                <Button size="large">
                  Дополнительная кнопка
                </Button>
              </div>
              
              <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mt-8">
                <Card className="text-center">
                  <ThunderboltOutlined className="text-4xl text-blue-500 mb-4" />
                  <Title level={4}>Быстрый старт</Title>
                  <Paragraph>Быстрое развертывание и настройка</Paragraph>
                </Card>
                
                <Card className="text-center">
                  <CloudOutlined className="text-4xl text-green-500 mb-4" />
                  <Title level={4}>Облачные ресурсы</Title>
                  <Paragraph>Управление всеми облачными сервисами</Paragraph>
                </Card>
                
                <Card className="text-center">
                  <SettingOutlined className="text-4xl text-purple-500 mb-4" />
                  <Title level={4}>Настройки</Title>
                  <Paragraph>Гибкая конфигурация системы</Paragraph>
                </Card>
              </div>
            </Card>
          </Col>
        </Row>
      </Content>
      
      <Footer className="text-center bg-gray-50">
        <Paragraph className="text-gray-500">
          Stroppy Cloud Panel ©2024 - Создано с помощью React, TypeScript, Vite, Tailwind CSS и Ant Design
        </Paragraph>
      </Footer>
    </Layout>
  )
}

export default App
