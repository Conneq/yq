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
	d        DataTreeNavigator
}

func NewTomlDecoder() Decoder {
	return &tomlDecoder{
		finished: false,
		d:        NewDataTreeNavigator(),
	}
}

func (dec *tomlDecoder) Init(reader io.Reader) error {
	dec.parser = toml.Parser{}
	buf := new(bytes.Buffer)
	buf.ReadFrom(reader)
	dec.parser.Reset(buf.Bytes())
	return nil
}

func (dec *tomlDecoder) getFullPath(tomlNode *toml.Node) ([]interface{}, error) {
	path := make([]interface{}, 0)
	for {
		path = append(path, string(tomlNode.Data))
		tomlNode = tomlNode.Next()
		if tomlNode == nil {
			return path, nil
		}
	}
}

func (dec *tomlDecoder) createKeyValueMap(tomlNode *toml.Node) (*yaml.Node, error) {

	value := tomlNode.Value()
	path, err := dec.getFullPath(value.Next())
	if err != nil {
		return nil, err
	}

	rootMap := &CandidateNode{
		Node: &yaml.Node{
			Kind: yaml.MappingNode,
			Tag:  "!!map",
		},
	}

	valueNode, err := dec.convertToYamlNode(value)
	if err != nil {
		return nil, err
	}
	context := Context{}
	context = context.SingleChildContext(rootMap)

	err = dec.d.DeeplyAssign(context, path, valueNode)
	if err != nil {
		return nil, err
	}

	return rootMap.Node, nil
}

func (dec *tomlDecoder) createTable(tomlNode *toml.Node) (*yaml.Node, error) {
	log.Debug("Table: %v", string(tomlNode.Data))
	iterator := tomlNode.Children()
	for iterator.Next() {
		child := iterator.Node()
		log.Debug("child: %v, %v", string(child.Data), child.Kind)

	}

	return &yaml.Node{
		Kind: yaml.MappingNode,
		Tag:  "!!map",
	}, nil
}

func (dec *tomlDecoder) createInlineTableMap(tomlNode *toml.Node) (*yaml.Node, error) {
	content := make([]*yaml.Node, 0)

	iterator := tomlNode.Children()
	for iterator.Next() {
		child := iterator.Node()
		if child.Kind != toml.KeyValue {
			return nil, fmt.Errorf("only keyvalue pairs are supported in inlinetables, got %v instead", child.Kind)
		}

		keyValues, err := dec.createKeyValueMap(child)
		if err != nil {
			return nil, err
		}
		content = append(content, keyValues.Content...)
	}

	return &yaml.Node{
		Kind:    yaml.MappingNode,
		Tag:     "!!map",
		Content: content,
	}, nil
}

func (dec *tomlDecoder) createArray(tomlNode *toml.Node) (*yaml.Node, error) {
	content := make([]*yaml.Node, 0)
	iterator := tomlNode.Children()
	for iterator.Next() {
		child := iterator.Node()
		yamlNode, err := dec.convertToYamlNode(child)
		if err != nil {
			return nil, err
		}
		content = append(content, yamlNode)
	}

	return &yaml.Node{
		Kind:    yaml.SequenceNode,
		Tag:     "!!seq",
		Content: content,
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
	case toml.Array:
		return dec.createArray(tomlNode)
	case toml.InlineTable:
		return dec.createInlineTableMap(tomlNode)
	case toml.Table:
		return dec.createTable(tomlNode)
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
