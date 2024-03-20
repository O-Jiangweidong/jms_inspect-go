package task

import (
    "fmt"
    "strconv"
    "strings"
    "time"

    "golang.org/x/crypto/ssh"

    "inspect/pkg/common"
)

type Machine struct {
    Name     string
    Type     string
    Host     string
    Port     string
    Username string
    Password string
    Valid    bool

    Client *ssh.Client `json:"-"`
}

func (m *Machine) Connect() bool {
    sshConfig := &ssh.ClientConfig{
        User:            m.Username,
        Auth:            []ssh.AuthMethod{ssh.Password(m.Password)},
        HostKeyCallback: ssh.InsecureIgnoreHostKey(),
        Timeout:         10 * time.Second,
    }
    address := fmt.Sprintf("%s:%s", m.Host, m.Port)
    if client, err := ssh.Dial("tcp", address, sshConfig); err != nil {
        return false
    } else {
        m.Client = client
        return true
    }
}

func (m *Machine) DoCommand(cmd string) (string, error) {
    session, err := m.Client.NewSession()
    if err != nil {
        return "", err
    }
    rest, err := session.CombinedOutput(cmd)
    if err != nil {
        return "", err
    }
    return strings.TrimSpace(string(rest)), nil
}

func (m *Machine) Down() {
    if m.Client != nil {
        _ = m.Client.Close()
    }
}

func (m *Machine) GetExecutor() *Executor {
    executor := Executor{Machine: m}
    executor.Tasks = m.GetTasks()
    return &executor
}

func (m *Machine) GetTasks() []AbstractTask {
    generalTasks := []AbstractTask{&OsInfoTask{Machine: m}}
    switch m.Type {
    case common.JumpServer:
        generalTasks = append(generalTasks, &ServiceTask{Machine: m})
    case common.Redis:
        generalTasks = append(generalTasks, &RedisTask{})
    case common.MySQL:
        generalTasks = append(generalTasks, &MySQLTask{})
    }
    return generalTasks
}

type AbnormalMsg struct {
    Level        string
    Desc         string
    NodeName     string
    LevelDisplay string
}

type AbstractTask interface {
    Init(options *Options) error
    GetName() string
    Run() error
    GetResult() (map[string]interface{}, []AbnormalMsg)
}

type Task struct {
    result         map[string]interface{}
    abnormalResult []AbnormalMsg

    Machine   *Machine
    Options   *Options
    JMSConfig map[string]string
}

type Executor struct {
    Machine *Machine
    Tasks   []AbstractTask

    Result         map[string]interface{}
    AbnormalResult []AbnormalMsg
    Logger         *common.Logger
}

func (e *Executor) Execute(opts *Options) (map[string]interface{}, []AbnormalMsg) {
    e.Logger.Info("开始执行机器名为[%s]的任务，共%v个", e.Machine.Name, len(e.Tasks))
    var err error
    e.Result = make(map[string]interface{})
    for _, t := range e.Tasks {
        start := time.Now()
        e.Logger.Info("开始执行任务：%s", t.GetName())
        err = t.Init(opts)
        if err != nil {
            e.Logger.Error("初始化任务失败: %s", err)
        }
        err = t.Run()
        if err != nil {
            e.Logger.Warning("执行任务出错: %s", err)
        }
        duration := strconv.FormatFloat(time.Now().Sub(start).Seconds(), 'f', 2, 64)
        e.MergeResult(t.GetResult())
        e.Logger.Info("执行结束（耗时：%s秒）", duration)
    }
    e.Machine.Down()
    e.Logger.Info("机器名为[%s]的任务全部执行结束\n", e.Machine.Name)
    return e.Result, e.AbnormalResult
}

func (e *Executor) MergeResult(result map[string]interface{}, abnormalResult []AbnormalMsg) {
    for key, value := range result {
        e.Result[key] = value
    }
    e.AbnormalResult = append(e.AbnormalResult, abnormalResult...)
}

func (t *Task) Init(opts *Options) error {
    t.Options = opts
    t.result = make(map[string]interface{})
    return nil
}

func (t *Task) GetConfig(key, input string) string {
    if v, exist := t.Options.JMSConfig[key]; exist {
        return v
    } else {
        return input
    }
}

func (t *Task) SetAbnormalEvent(desc, level string) {
    displayMap := make(map[string]string)
    displayMap[common.Critical] = "严重"
    displayMap[common.NORMAL] = "一般"
    displayMap[common.SLIGHT] = "轻微"

    t.abnormalResult = append(t.abnormalResult, AbnormalMsg{
        Level: level, Desc: desc, LevelDisplay: displayMap[level],
    })
}

func (t *Task) GetResult() (map[string]interface{}, []AbnormalMsg) {
    return t.result, t.abnormalResult
}
