package yqlib

import (
	"bytes"
	"fmt"
	"io"
	"strconv"

	toml "github.com/pelletier/go-toml/v2/unstable"
	yaml "gopkg.in/yaml.v3"
)

type tomlDecoder struct {
	parser   toml.Parser
	finished bool
}

func NewTomlDecoder() Decoder {
	return &tomlDecoder{
		finished: false,
	}
}

func (dec *tomlDecoder) Init(reader io.Reader) error {
	dec.parser = toml.Parser{}
	buf := new(bytes.Buffer)
	buf.ReadFrom(reader)
	dec.parser.Reset(buf.Bytes())
	return nil
}

func (dec *tomlDecoder) createKeyValueMap(tomlNode *toml.Node) (*yaml.Node, error) {

	value := tomlNode.Value()
	key := value.Next()

	keyNode, err := dec.convertToYamlNode(key)
	if err != nil {
		return nil, err
	}
	valueNode, err := dec.convertToYamlNode(value)
	if err != nil {
		return nil, err
	}

	return &yaml.Node{
		Kind:    yaml.MappingNode,
		Content: []*yaml.Node{keyNode, valueNode},
	}, nil

}

func (dec *tomlDecoder) createStringScalar(tomlNode *toml.Node) (*yaml.Node, error) {
	content := string(tomlNode.Data)
	return createScalarNode(content, content), nil
}

func (dec *tomlDecoder) createBoolScalar(tomlNode *toml.Node) (*yaml.Node, error) {
	content := string(tomlNode.Data)
	return createScalarNode(content == "true", content), nil
}

func (dec *tomlDecoder) createIntegerScalar(tomlNode *toml.Node) (*yaml.Node, error) {
	content := string(tomlNode.Data)
	_, num, err := parseInt64(content)
	return createScalarNode(num, content), err
}

func (dec *tomlDecoder) createFloatScalar(tomlNode *toml.Node) (*yaml.Node, error) {
	content := string(tomlNode.Data)
	num, err := strconv.ParseFloat(content, 64)
	return createScalarNode(num, content), err
}

func (dec *tomlDecoder) convertToYamlNode(tomlNode *toml.Node) (*yaml.Node, error) {
	switch tomlNode.Kind {
	case toml.KeyValue:
		return dec.createKeyValueMap(tomlNode)
	case toml.Key, toml.String:
		return dec.createStringScalar(tomlNode)
	case toml.Bool:
		return dec.createBoolScalar(tomlNode)
	case toml.Integer:
		return dec.createIntegerScalar(tomlNode)
	case toml.Float:
		return dec.createFloatScalar(tomlNode)
	default:
		return nil, fmt.Errorf("unsupported type %v", tomlNode.Kind)
	}
}

func (dec *tomlDecoder) Decode() (*CandidateNode, error) {
	// if dec.finished {
	// 	return nil, io.EOF
	// }
	//
	// toml library likes to panic
	var deferredError error
	defer func() { //catch or finally
		if r := recover(); r != nil {
			var ok bool
			deferredError, ok = r.(error)
			if !ok {
				deferredError = fmt.Errorf("pkg: %v", r)
			}
		}
	}()

	readData := dec.parser.NextExpression()

	err := dec.parser.Error()
	if err != nil {
		return nil, err
	}

	// must have finished
	if !readData {
		dec.finished = true
		return nil, io.EOF
	}

	result := dec.parser.Expression()

	firstNode, err := dec.convertToYamlNode(result)
	if err != nil {
		return nil, err
	}

	return &CandidateNode{
		Node: &yaml.Node{
			Kind:    yaml.DocumentNode,
			Content: []*yaml.Node{firstNode},
		},
	}, deferredError

}
