package core

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// ConnectionPool 负责管理 SQLite 数据库连接的生命周期。
// 它保证同一个文件只会被打开一次，并且是线程安全的。
type ConnectionPool struct {
	mu      sync.RWMutex
	connMap map[string]*sql.DB // 路径 -> 连接对象
	baseDir string             // 基础工作目录
}

// NewConnectionPool 创建一个新的连接池
func NewConnectionPool(baseDir string) *ConnectionPool {
	return &ConnectionPool{
		connMap: make(map[string]*sql.DB),
		baseDir: baseDir,
	}
}

// GetConnection 获取指定路径的数据库连接。
// 如果连接已存在且活跃，直接返回；否则创建新连接。
func (p *ConnectionPool) GetConnection(path string) (*sql.DB, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 1. 尝试从缓存获取
	if conn, ok := p.connMap[path]; ok {
		if err := conn.Ping(); err == nil {
			return conn, nil
		}
		// 连接失效，清理旧连接
		_ = conn.Close()
		delete(p.connMap, path)
	}

	// 2. 建立新连接
	conn, err := p.openNewConnection(path)
	if err != nil {
		return nil, err
	}

	p.connMap[path] = conn
	return conn, nil
}

// openNewConnection 封装底层的 SQL 打开逻辑 (单一职责：创建)
func (p *ConnectionPool) openNewConnection(path string) (*sql.DB, error) {
	// 不要用 cache=shared —— 共享缓存模式下 SQLite 用表级锁把并发读也串行化了，
	// 年度报告那种多个分区同时扫同一批表的场景会被卡成单线程。
	// busy_timeout 让偶发的锁竞争自动重试，而不是立刻报 SQLITE_BUSY。
	dsn := fmt.Sprintf("file:%s?mode=rw&_busy_timeout=5000", path)

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("无法打开数据库文件 %s: %w", path, err)
	}

	// 允许若干条并发读连接。默认不限反而容易在高并发下开出过多 fd，
	// 这里给一个够用又克制的上限。
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	db.SetConnMaxIdleTime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("数据库文件 %s 连接测试失败: %w", path, err)
	}

	return db, nil
}

// CloseConnection 关闭并移除特定路径的连接
func (p *ConnectionPool) CloseConnection(path string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if conn, ok := p.connMap[path]; ok {
		err := conn.Close()
		delete(p.connMap, path)
		return err
	}
	return nil
}

// CloseAll 关闭池中所有连接
func (p *ConnectionPool) CloseAll() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var errs []error
	for path, conn := range p.connMap {
		if err := conn.Close(); err != nil {
			errs = append(errs, fmt.Errorf("关闭 %s 失败: %w", path, err))
		}
	}
	// 重置 map
	p.connMap = make(map[string]*sql.DB)

	if len(errs) > 0 {
		return fmt.Errorf("关闭连接池时出现错误: %v", errs)
	}
	return nil
}
