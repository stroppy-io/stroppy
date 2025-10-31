import React, { useState, useEffect } from 'react'
import { Button, Space, Typography, Dropdown, Drawer, Menu, Switch } from 'antd'
import { 
  GlobalOutlined,
  HomeOutlined,
  SearchOutlined,
  TranslationOutlined,
  MenuOutlined,
  UserOutlined,
  EyeOutlined
} from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { useTheme } from '../../common/contexts/ThemeContext'
import { useTranslation } from '../../common/hooks/useTranslation'

const { Title } = Typography

interface HeaderProps {
  showSearchButton?: boolean
  showHomeButton?: boolean
  showDocsButton?: boolean
  onSearchClick?: () => void
}

const Header: React.FC<HeaderProps> = ({
  showSearchButton = false,
  showHomeButton = false,
  showDocsButton = false,
  onSearchClick
}) => {
  const navigate = useNavigate()
  const { darkReaderEnabled, toggleDarkReader } = useTheme()
  const { changeLanguage, getCurrentLanguage, getAvailableLanguages, t } = useTranslation('landing')
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false)
  const [isMobile, setIsMobile] = useState(false)

  useEffect(() => {
    const checkIsMobile = () => {
      setIsMobile(window.innerWidth < 768)
    }
    
    checkIsMobile()
    window.addEventListener('resize', checkIsMobile)
    
    return () => window.removeEventListener('resize', checkIsMobile)
  }, [])

  const handleLanguageChange = (languageCode: string) => {
    changeLanguage(languageCode)
  }

  const handleMobileMenuClose = () => {
    setMobileMenuOpen(false)
  }

  return (
    <>
      <div 
        style={{ 
          background: darkReaderEnabled ? '#1f1f1f' : '#fafafa', 
          padding: isMobile ? '8px 8px' : '8px 24px',
          boxShadow: darkReaderEnabled ? '0 2px 8px rgba(0,0,0,0.2)' : '0 2px 8px rgba(0,0,0,0.05)',
          position: 'fixed',
          top: 0,
          left: 0,
          right: 0,
          zIndex: 1000,
          width: '100%',
          minWidth: '320px',
          height: isMobile ? '56px' : '60px',
          display: 'flex',
          alignItems: 'center',
          borderBottom: darkReaderEnabled ? '1px solid #333' : '1px solid #e8e8e8',
          boxSizing: 'border-box',
          overflow: 'hidden'
        }}
      >
        <div style={{ 
          display: 'flex', 
          alignItems: 'center', 
          justifyContent: 'space-between',
          maxWidth: isMobile ? '100%' : '1200px',
          margin: '0 auto',
          width: '100%',
          minWidth: '0',
          flex: '1 1 auto',
          boxSizing: 'border-box'
        }}>
          {/* Логотип */}
          <div 
            style={{ 
              display: 'flex', 
              alignItems: 'center', 
              gap: isMobile ? '8px' : '12px', 
              flex: '0 0 auto', 
              padding: '8px 0',
              cursor: 'pointer',
              transition: 'opacity 0.3s ease',
              minWidth: '0',
              overflow: 'hidden'
            }}
            onClick={() => navigate('/')}
            onMouseEnter={(e) => e.currentTarget.style.opacity = '0.8'}
            onMouseLeave={(e) => e.currentTarget.style.opacity = '1'}
          >
            <div style={{
              width: isMobile ? '28px' : '36px',
              height: isMobile ? '28px' : '36px',
              background: 'linear-gradient(135deg, #1890ff, #096dd9)',
              borderRadius: '6px',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center'
            }}>
              <GlobalOutlined style={{ color: 'white', fontSize: isMobile ? '14px' : '18px' }} />
            </div>
            <Title 
              level={3} 
              style={{ 
                margin: 0, 
                color: darkReaderEnabled ? '#ffffff' : '#595959',
                fontSize: isMobile ? '12px' : '18px',
                display: 'block',
                whiteSpace: 'nowrap',
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                maxWidth: isMobile ? '120px' : 'none',
                flex: isMobile ? '0 0 auto' : 'none'
              }}
            >
              {isMobile ? 'Stroppy' : 'Stroppy Cloud Panel'}
            </Title>
          </div>
          
          {/* Навигация - десктоп */}
          <div style={{ 
            flex: '0 0 auto', 
            padding: '8px 0',
            display: isMobile ? 'none' : 'flex',
            alignItems: 'center',
            minWidth: '0',
            overflow: 'hidden'
          }}>
            <Space size={isMobile ? "small" : "middle"}>
            {/* Кнопка "Главная" - показывается только на странице документации */}
            {showHomeButton && (
              <Button 
                type="text" 
                onClick={() => navigate('/')}
                icon={<HomeOutlined />}
                style={{ 
                  color: darkReaderEnabled ? '#ffffff' : '#595959',
                  fontWeight: '500',
                  fontSize: isMobile ? '12px' : '14px',
                  borderRadius: '6px',
                  padding: isMobile ? '4px 8px' : '6px 12px',
                  background: darkReaderEnabled ? 'rgba(255, 255, 255, 0.08)' : 'rgba(0, 0, 0, 0.03)',
                  border: '1px solid transparent',
                  transition: 'all 0.3s ease'
                }}
              >
                {t('navigation.home')}
              </Button>
            )}

            {/* Кнопка поиска - показывается только на странице документации */}
            {showSearchButton && onSearchClick && (
              <Button 
                type="text" 
                onClick={onSearchClick}
                style={{ 
                  color: darkReaderEnabled ? '#ffffff' : '#595959',
                  fontWeight: '500',
                  fontSize: isMobile ? '12px' : '14px',
                  borderRadius: '6px',
                  padding: isMobile ? '4px 8px' : '6px 12px',
                  background: darkReaderEnabled ? 'rgba(255, 255, 255, 0.08)' : 'rgba(0, 0, 0, 0.03)',
                  border: '1px solid transparent',
                  transition: 'all 0.3s ease',
                  display: 'flex',
                  alignItems: 'center',
                  gap: '8px'
                }}
              >
                <SearchOutlined />
                <span>{t('documentation.search.title')}</span>
                <span style={{ 
                  fontSize: '11px', 
                  opacity: 0.7,
                  background: darkReaderEnabled ? 'rgba(255, 255, 255, 0.1)' : 'rgba(0, 0, 0, 0.1)',
                  padding: '2px 6px',
                  borderRadius: '4px',
                  fontFamily: 'monospace'
                }}>
                  Ctrl+K
                </span>
              </Button>
            )}

            {/* Кнопка "Документация" - показывается только на главной странице */}
            {showDocsButton && (
              <Button 
                type="text" 
                onClick={() => navigate('/docs')}
                style={{ 
                  color: darkReaderEnabled ? '#ffffff' : '#595959',
                  fontWeight: '500',
                  fontSize: isMobile ? '12px' : '14px',
                  borderRadius: '6px',
                  padding: isMobile ? '4px 8px' : '6px 12px',
                  background: darkReaderEnabled ? 'rgba(255, 255, 255, 0.08)' : 'rgba(0, 0, 0, 0.03)',
                  border: '1px solid transparent',
                  transition: 'all 0.3s ease'
                }}
              >
                {t('footer.links.documentation')}
              </Button>
            )}

            {/* Переключатель темы */}
            <div style={{ 
              display: 'flex', 
              alignItems: 'center', 
              gap: '8px',
              padding: isMobile ? '4px 8px' : '6px 12px',
              background: darkReaderEnabled ? 'rgba(255, 255, 255, 0.08)' : 'rgba(0, 0, 0, 0.03)',
              borderRadius: '6px',
              border: '1px solid transparent'
            }}>
              <EyeOutlined style={{ 
                color: darkReaderEnabled ? '#ffffff' : '#595959', 
                fontSize: isMobile ? '12px' : '14px' 
              }} />
              <Switch
                size="small"
                checked={darkReaderEnabled}
                onChange={toggleDarkReader}
                checkedChildren="🌙"
                unCheckedChildren="☀️"
              />
            </div>

            {/* Переключатель языка */}
            <Dropdown
              menu={{
                items: getAvailableLanguages().map((lang) => ({
                  key: lang.code,
                  label: lang.nativeName,
                  onClick: () => handleLanguageChange(lang.code),
                  icon: <TranslationOutlined />
                }))
              }}
              placement="bottomRight"
              trigger={['click']}
            >
              <Button 
                type="text" 
                icon={<TranslationOutlined />}
                style={{ 
                  color: darkReaderEnabled ? '#ffffff' : '#595959',
                  display: 'flex',
                  alignItems: 'center',
                  gap: '4px',
                  fontWeight: '500',
                  fontSize: isMobile ? '12px' : '14px',
                  borderRadius: '6px',
                  padding: isMobile ? '4px 8px' : '6px 12px',
                  background: darkReaderEnabled ? 'rgba(255, 255, 255, 0.08)' : 'rgba(0, 0, 0, 0.03)',
                  border: '1px solid transparent',
                  transition: 'all 0.3s ease'
                }}
              >
                {getCurrentLanguage().toUpperCase()}
              </Button>
            </Dropdown>

            {/* Кнопки входа и регистрации - показываются на всех страницах */}
            <Button 
              type="text" 
              onClick={() => navigate('/login')}
              style={{ 
                color: darkReaderEnabled ? '#ffffff' : '#595959',
                fontWeight: '500',
                borderRadius: '6px',
                padding: '6px 12px',
                background: darkReaderEnabled ? 'rgba(255, 255, 255, 0.08)' : 'rgba(0, 0, 0, 0.03)',
                border: '1px solid transparent',
                transition: 'all 0.3s ease'
              }}
            >
              {t('navigation.login')}
            </Button>
            
            <Button 
              type="primary"
              onClick={() => navigate('/register')}
              style={{ 
                fontWeight: '500',
                borderRadius: '6px',
                padding: '6px 12px',
                transition: 'all 0.3s ease'
              }}
            >
              {t('navigation.getStarted')}
            </Button>
            </Space>
          </div>

          {/* Мобильное меню */}
          <div style={{ 
            display: isMobile ? 'flex' : 'none',
            alignItems: 'center',
            flex: '0 0 auto',
            minWidth: '0'
          }}>
            <Button 
              type="text" 
              icon={<MenuOutlined />}
              onClick={() => setMobileMenuOpen(true)}
              style={{ 
                color: darkReaderEnabled ? '#ffffff' : '#595959',
                fontSize: '18px',
                padding: '8px'
              }}
            />
          </div>
        </div>
      </div>

      {/* Мобильное меню Drawer */}
      <Drawer
        title={
          <div style={{ 
            display: 'flex', 
            alignItems: 'center', 
            gap: '12px' 
          }}>
            <div style={{
              width: '32px',
              height: '32px',
              background: 'linear-gradient(135deg, #1890ff, #096dd9)',
              borderRadius: '6px',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center'
            }}>
              <GlobalOutlined style={{ color: 'white', fontSize: '16px' }} />
            </div>
            <span style={{ fontSize: '16px', fontWeight: '600' }}>
              Stroppy Cloud Panel
            </span>
          </div>
        }
        placement="right"
        onClose={handleMobileMenuClose}
        open={mobileMenuOpen}
        width={280}
        styles={{
          body: {
            padding: '16px'
          }
        }}
      >
        <Menu
          mode="vertical"
          style={{ 
            border: 'none',
            background: 'transparent'
          }}
          items={[
            ...(showHomeButton ? [{
              key: 'home',
              icon: <HomeOutlined />,
              label: t('navigation.home'),
              onClick: () => {
                navigate('/')
                handleMobileMenuClose()
              }
            }] : []),
            ...(showSearchButton && onSearchClick ? [{
              key: 'search',
              icon: <SearchOutlined />,
              label: t('documentation.search.title'),
              onClick: () => {
                onSearchClick()
                handleMobileMenuClose()
              }
            }] : []),
            ...(showDocsButton ? [{
              key: 'docs',
              icon: <GlobalOutlined />,
              label: t('footer.links.documentation'),
              onClick: () => {
                navigate('/docs')
                handleMobileMenuClose()
              }
            }] : []),
            {
              key: 'divider1',
              type: 'divider'
            },
            {
              key: 'theme',
              icon: <EyeOutlined />,
              label: t('settings.theme.toggle'),
              onClick: () => {
                toggleDarkReader()
                handleMobileMenuClose()
              },
              extra: (
                <Switch
                  size="small"
                  checked={darkReaderEnabled}
                  onChange={toggleDarkReader}
                  checkedChildren="🌙"
                  unCheckedChildren="☀️"
                />
              )
            },
            {
              key: 'language',
              icon: <TranslationOutlined />,
              label: t('settings.language.ru') === 'Русский' ? 'Язык' : 'Language',
              children: getAvailableLanguages().map((lang) => ({
                key: lang.code,
                label: lang.nativeName,
                onClick: () => {
                  handleLanguageChange(lang.code)
                  handleMobileMenuClose()
                }
              }))
            },
            {
              key: 'divider2',
              type: 'divider'
            },
            {
              key: 'login',
              icon: <UserOutlined />,
              label: t('navigation.login'),
              onClick: () => {
                navigate('/login')
                handleMobileMenuClose()
              }
            },
            {
              key: 'register',
              icon: <UserOutlined />,
              label: t('navigation.getStarted'),
              onClick: () => {
                navigate('/register')
                handleMobileMenuClose()
              },
              style: {
                background: '#1890ff',
                color: 'white',
                borderRadius: '6px',
                marginTop: '8px'
              }
            }
          ]}
        />
      </Drawer>
    </>
  )
}

export default Header
