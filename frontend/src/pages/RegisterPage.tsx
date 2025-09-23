import React, { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { 
  Card, 
  Form, 
  Input, 
  Button, 
  Typography, 
  Alert, 
  Layout
} from 'antd';
import { 
  UserOutlined, 
  LockOutlined, 
  CloudOutlined
} from '@ant-design/icons';
import { useAuth } from '../contexts/AuthContext';
import { getErrorMessage } from '../services/api';

const { Title, Text } = Typography;

const RegisterPage: React.FC = () => {
  const [error, setError] = useState('');
  const [isLoading, setIsLoading] = useState(false);

  const { register } = useAuth();
  const navigate = useNavigate();

  const handleSubmit = async (values: { 
    username: string; 
    password: string; 
    confirmPassword: string; 
  }) => {
    setError('');

    if (values.password !== values.confirmPassword) {
      setError('Пароли не совпадают');
      return;
    }

    try {
      setIsLoading(true);
      await register({
        username: values.username.trim(),
        password: values.password,
      });
      navigate('/');
    } catch (err) {
      setError(getErrorMessage(err));
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <Layout style={{ minHeight: '100vh', background: 'linear-gradient(135deg, #667eea 0%, #764ba2 100%)' }}>
        <div style={{
          display: 'flex',
          justifyContent: 'center',
          alignItems: 'center',
          minHeight: '100vh',
          padding: '24px',
          position: 'relative'
        }}>

          <Card
            style={{
              width: '100%',
              maxWidth: '400px',
              boxShadow: '0 8px 32px rgba(0,0,0,0.12)'
            }}
          >
            <div style={{ textAlign: 'center', marginBottom: '32px' }}>
              <div style={{
                width: '64px',
                height: '64px',
                background: 'linear-gradient(135deg, #1890ff, #096dd9)',
                borderRadius: '16px',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                margin: '0 auto 16px'
              }}>
                <CloudOutlined style={{ color: 'white', fontSize: '32px' }} />
              </div>
              
              <Title level={2} style={{ margin: '0 0 8px 0' }}>
                Создание аккаунта
              </Title>
              
              <Text type="secondary">
                Создайте новый аккаунт для доступа к системе
              </Text>
            </div>

            {error && (
              <Alert
                message={error}
                type="error"
                showIcon
                style={{ marginBottom: '24px' }}
              />
            )}

            <Form
              name="register"
              onFinish={handleSubmit}
              layout="vertical"
              size="large"
            >
              <Form.Item
                name="username"
                label="Имя пользователя"
                rules={[
                  { required: true, message: 'Пожалуйста, введите имя пользователя!' },
                  { min: 3, message: 'Имя пользователя должно содержать минимум 3 символа!' },
                  { max: 50, message: 'Имя пользователя не должно превышать 50 символов!' },
                  { 
                    pattern: /^[a-zA-Z0-9_-]+$/, 
                    message: 'Имя пользователя может содержать только буквы, цифры, дефисы и подчеркивания!' 
                  }
                ]}
              >
                <Input
                  prefix={<UserOutlined />}
                  placeholder="Введите имя пользователя"
                  disabled={isLoading}
                />
              </Form.Item>

              <Form.Item
                name="password"
                label="Пароль"
                rules={[
                  { required: true, message: 'Пожалуйста, введите пароль!' },
                  { min: 6, message: 'Пароль должен содержать минимум 6 символов!' }
                ]}
              >
                <Input.Password
                  prefix={<LockOutlined />}
                  placeholder="Введите пароль"
                  disabled={isLoading}
                />
              </Form.Item>

              <Form.Item
                name="confirmPassword"
                label="Подтверждение пароля"
                dependencies={['password']}
                rules={[
                  { required: true, message: 'Пожалуйста, подтвердите пароль!' },
                  ({ getFieldValue }) => ({
                    validator(_, value) {
                      if (!value || getFieldValue('password') === value) {
                        return Promise.resolve();
                      }
                      return Promise.reject(new Error('Пароли не совпадают!'));
                    },
                  }),
                ]}
              >
                <Input.Password
                  prefix={<LockOutlined />}
                  placeholder="Повторите пароль"
                  disabled={isLoading}
                />
              </Form.Item>

              <Form.Item style={{ marginBottom: '16px' }}>
                <Button
                  type="primary"
                  htmlType="submit"
                  loading={isLoading}
                  block
                  style={{ height: '44px', fontSize: '16px' }}
                >
                  Создать аккаунт
                </Button>
              </Form.Item>
            </Form>

            <div style={{ textAlign: 'center', marginTop: '24px' }}>
              <Text type="secondary">
                Уже есть аккаунт?{' '}
                <Link to="/login" style={{ fontWeight: 500 }}>
                  Войти в систему
                </Link>
              </Text>
            </div>
          </Card>
        </div>
      </Layout>
  );
};

export default RegisterPage;