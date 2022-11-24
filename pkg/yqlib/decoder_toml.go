package yqlib

import (
	"bytes"
	"errors"
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
	_, err := buf.ReadFrom(reader)
	if err != nil {
		return err
	}
	dec.parser.Reset(buf.Bytes())
	return nil
}

func (dec *tomlDecoder) getFullPath(tomlNode *toml.Node) []interface{} {
	path := make([]interface{}, 0)
	for {
		path = append(path, string(tomlNode.Data))
		tomlNode = tomlNode.Next()
		if tomlNode == nil {
			return path
		}
	}
}

func (dec *tomlDecoder) processKeyValueIntoMap(rootMap *CandidateNode, tomlNode *toml.Node) error {
	value := tomlNode.Value()
	path := dec.getFullPath(value.Next())

	valueNode, err := dec.decodeNode(value)
	if err != nil {
		return err
	}
	context := Context{}
	context = context.SingleChildContext(rootMap)

	return dec.d.DeeplyAssign(context, path, valueNode)
}

func (dec *tomlDecoder) decodeKeyValuesIntoMap(rootMap *CandidateNode, tomlNode *toml.Node) error {
	log.Debug("!! DECODE_KV_INTO_MAP -- processing first (current) entry")
	if err := dec.processKeyValueIntoMap(rootMap, tomlNode); err != nil {
		return err
	}

	for dec.parser.NextExpression() {
		nextItem := dec.parser.Expression()
		log.Debug("!! DECODE_KV_INTO_MAP -- next exp, its a %v", nextItem.Kind)

		if nextItem.Kind == toml.KeyValue {
			if err := dec.processKeyValueIntoMap(rootMap, nextItem); err != nil {
				return err
			}
		} else {
			// run out of key values
			log.Debug("! DECODE_KV_INTO_MAP - ok we are done in decodeKeyValuesIntoMap, gota a %v", nextItem.Kind)
			return nil
		}
	}
	log.Debug("! DECODE_KV_INTO_MAP - no more things to read in %w", dec.parser.Error())
	if dec.parser.Error() != nil {
		return dec.parser.Error()
	}
	return io.EOF
}

func (dec *tomlDecoder) createKeyValueMap(tomlNode *toml.Node) (*yaml.Node, error) {

	rootMap := &CandidateNode{
		Node: &yaml.Node{
			Kind: yaml.MappingNode,
			Tag:  "!!map",
		},
	}
	err := dec.decodeKeyValuesIntoMap(rootMap, tomlNode)
	log.Debug("! createKeyValueMap done, %v ", NodeToString(rootMap))
	return rootMap.Node, err
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
		yamlNode, err := dec.decodeNode(child)
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

func (dec *tomlDecoder) decodeNode(tomlNode *toml.Node) (*yaml.Node, error) {
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
	default:
		return nil, fmt.Errorf("unsupported type %v", tomlNode.Kind)
	}

}

func (dec *tomlDecoder) Decode() (*CandidateNode, error) {
	if dec.finished {
		return nil, io.EOF
	}
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

	log.Debug("ok here we go")
	newMap := &CandidateNode{
		Node: &yaml.Node{
			Kind: yaml.MappingNode,
			Tag:  "!!map",
		}}

	var currentNode *toml.Node = nil

	for (currentNode != nil && currentNode != dec.parser.Expression()) || dec.parser.NextExpression() {

		currentNode = dec.parser.Expression()

		log.Debug("currentNode: %v ", currentNode.Kind)

		if currentNode.Kind == toml.Table {
			log.Debug("!!! processing table")
			fullPath := dec.getFullPath(currentNode.Child())
			log.Debug("!!!fullpath: %v", fullPath)

			hasValue := dec.parser.NextExpression() // get the value of the table
			if !hasValue {
				return nil, fmt.Errorf("error retrieving table %v value: %w", fullPath, dec.parser.Error())
			}

			// now we expect to get a sequence of key/value pairs

			tableValue := dec.parser.Expression()
			tableNodeValue, err := dec.decodeNode(tableValue)
			if err != nil {
				return nil, err
			}
			log.Debugf("table node %v", tableNodeValue.Tag)
			c := Context{}

			c = c.SingleChildContext(newMap)
			err = dec.d.DeeplyAssign(c, fullPath, tableNodeValue)
			if err != nil {
				return nil, err
			}

		} else {

			err := dec.decodeKeyValuesIntoMap(newMap, currentNode)
			log.Debug("TOP LEVEL KV DONE %v", NodeToString(newMap))
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				log.Debug("wait waht")
				return nil, err
			}
			log.Debug("next exp %v vs %v", currentNode, dec.parser.Expression())
		}
	}

	err := dec.parser.Error()
	if err != nil {
		return nil, err
	}

	// must have finished
	dec.finished = true

	if len(newMap.Node.Content) == 0 {
		return nil, io.EOF
	}

	return &CandidateNode{
		Node: &yaml.Node{
			Kind:    yaml.DocumentNode,
			Content: []*yaml.Node{newMap.Node},
		},
	}, deferredError

}
